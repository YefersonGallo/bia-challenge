package live

import (
	"fmt"
	"sort"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// AlertState is the lifecycle of a live alert.
type AlertState string

// Live alert states.
const (
	AlertCandidate AlertState = "CANDIDATE" // detected, waiting to persist
	AlertConfirmed AlertState = "CONFIRMED" // persisted ConfirmAfter hours
	AlertClosed    AlertState = "CLOSED"    // back to normal, explained or discarded
)

// Alert is what the operator sees in the live view.
type Alert struct {
	ID          string             `json:"id"`
	MeterID     string             `json:"meter_id"`
	State       AlertState         `json:"state"`
	Type        domain.AnomalyType `json:"type"`
	Severity    domain.Severity    `json:"severity"`
	Confidence  float64            `json:"confidence"`
	Priority    float64            `json:"priority_score"`
	Reason      string             `json:"reason"`
	ChangeAt    *time.Time         `json:"change_point_at,omitempty"`
	OpenedAt    time.Time          `json:"opened_at"`
	ConfirmedAt *time.Time         `json:"confirmed_at,omitempty"`
	UpdatedAt   time.Time          `json:"updated_at"`
	ClosedAt    *time.Time         `json:"closed_at,omitempty"`
	Closure     string             `json:"closure,omitempty"`
}

// AlertChange is one transition, published as an "alert" message.
type AlertChange struct {
	Change string `json:"change"` // candidate | confirmed | updated | closed
	Alert  Alert  `json:"alert"`
}

// Manager turns the findings of each tick into alert transitions. A finding opens a
// candidate; it is confirmed when it persists ConfirmAfter; a confirmed alert is
// updated when its type or severity changes, and closed when the finding disappears
// or the engine re-classifies it as a false positive.
type Manager struct {
	ConfirmAfter time.Duration
	active       map[string]*Alert // by meter
	closed       []Alert
	seq          int
}

// NewManager with the confirmation delay (the plan uses 3 simulated hours).
func NewManager(confirmAfter time.Duration) *Manager {
	return &Manager{ConfirmAfter: confirmAfter, active: map[string]*Alert{}}
}

// Update applies the findings observed at `now` and returns the transitions.
func (m *Manager) Update(now time.Time, findings []analysis.Finding) []AlertChange {
	var out []AlertChange
	seen := map[string]bool{}
	for _, f := range findings {
		a := f.Anomaly
		seen[a.MeterID] = true
		cur := m.active[a.MeterID]
		if a.Type == domain.FalsePositive {
			if cur != nil {
				cur.Type, cur.Severity, cur.Confidence, cur.Reason = a.Type, a.Severity, a.Confidence, a.Reason
				out = append(out, m.close(cur, now, "explicada por un evento y el consumo se recuperó"))
			}
			continue
		}
		if cur == nil {
			m.seq++
			cur = &Alert{ID: fmt.Sprintf("L-%s-%d", a.MeterID, m.seq), MeterID: a.MeterID, State: AlertCandidate, OpenedAt: now}
			fill(cur, a, now)
			m.active[a.MeterID] = cur
			out = append(out, AlertChange{Change: "candidate", Alert: *cur})
			continue
		}
		changed := cur.Type != a.Type || cur.Severity != a.Severity
		fill(cur, a, now)
		switch {
		case cur.State == AlertCandidate && now.Sub(cur.OpenedAt) >= m.ConfirmAfter:
			t := now
			cur.State, cur.ConfirmedAt = AlertConfirmed, &t
			out = append(out, AlertChange{Change: "confirmed", Alert: *cur})
		case cur.State == AlertConfirmed && changed:
			out = append(out, AlertChange{Change: "updated", Alert: *cur})
		}
	}
	for meter, cur := range m.active {
		if !seen[meter] {
			why := "volvió a la normalidad"
			if cur.State == AlertCandidate {
				why = "descartada: no persistió"
			}
			out = append(out, m.close(cur, now, why))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Alert.MeterID < out[j].Alert.MeterID })
	return out
}

func fill(dst *Alert, a domain.Anomaly, now time.Time) {
	dst.Type, dst.Severity, dst.Confidence, dst.Priority, dst.Reason = a.Type, a.Severity, a.Confidence, a.PriorityScore, a.Reason
	dst.ChangeAt = a.ChangePointAt
	dst.UpdatedAt = now
}

func (m *Manager) close(a *Alert, now time.Time, why string) AlertChange {
	t := now
	a.State, a.ClosedAt, a.Closure, a.UpdatedAt = AlertClosed, &t, why, now
	delete(m.active, a.MeterID)
	m.closed = append(m.closed, *a)
	return AlertChange{Change: "closed", Alert: *a}
}

// Alerts returns the open alerts (by priority) followed by the closed ones (newest first).
func (m *Manager) Alerts() []Alert {
	out := make([]Alert, 0, len(m.active)+len(m.closed))
	for _, a := range m.active {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].State != out[j].State {
			return out[i].State == AlertConfirmed
		}
		return out[i].Priority > out[j].Priority
	})
	for i := len(m.closed) - 1; i >= 0; i-- {
		out = append(out, m.closed[i])
	}
	return out
}

// Active returns the open alert of a meter, if any.
func (m *Manager) Active(meterID string) *Alert { return m.active[meterID] }
