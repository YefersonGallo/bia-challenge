package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Contribution is how much a meter moved the plant total.
type Contribution struct {
	MeterID  string  `json:"meter_id"`
	Name     string  `json:"name"`
	DeltaKWh float64 `json:"delta_kwh"`
}

// PlanItem is one line of the action plan.
type PlanItem struct {
	MeterID  string             `json:"meter_id"`
	Type     domain.AnomalyType `json:"type"`
	Action   string             `json:"action"`
	Owner    string             `json:"owner"`
	Deadline string             `json:"deadline"`
	Steps    []string           `json:"steps"`
}

// Report is the document generated from the latest completed analysis.
type Report struct {
	RunID         string          `json:"run_id"`
	GeneratedAt   string          `json:"generated_at"`
	Headline      string          `json:"headline"`
	Summary       Summary         `json:"summary"`
	Contributions []Contribution  `json:"contributions"`
	RuleFlags     []MeterSummary  `json:"rule_flags"`
	Findings      []AnomalyDetail `json:"findings"`
	Plan          []PlanItem      `json:"plan"`
	Methodology   Methodology     `json:"methodology"`
	Sources       []string        `json:"sources"`
}

// Methodology exposes the engine thresholds used by the report with stable JSON names.
type Methodology struct {
	BaselineDays      int     `json:"baseline_days"`
	ShiftThresholdPct float64 `json:"shift_threshold_pct"`
	MinEpisodeHours   int     `json:"min_episode_hours"`
	VoltageTolPct     float64 `json:"voltage_tol_pct"`
	PFJump            float64 `json:"pf_jump"`
	CriticalPct       float64 `json:"critical_pct"`
	HighPct           float64 `json:"high_pct"`
	ZThreshold        float64 `json:"z_threshold"`
	EventWindowHours  float64 `json:"event_window_hours"`
	CoherenceTolPct   float64 `json:"coherence_tol_pct"`
	MinInvalid        int     `json:"min_invalid"`
	MinCoherencePct   float64 `json:"min_coherence_pct"`
	PFDropThreshold   float64 `json:"pf_drop_threshold"`
	NightRatioSignal  float64 `json:"night_ratio_signal"`
}

func methodologyOf(c analysis.Config) Methodology {
	return Methodology{
		BaselineDays:      c.BaselineDays,
		ShiftThresholdPct: c.ShiftThreshold * 100,
		MinEpisodeHours:   c.MinEpisodeHours,
		VoltageTolPct:     c.VoltageTol * 100,
		PFJump:            c.PFJump,
		CriticalPct:       c.CriticalPct,
		HighPct:           c.HighPct,
		ZThreshold:        c.ZThreshold,
		EventWindowHours:  c.EventWindow.Hours(),
		CoherenceTolPct:   c.CoherenceTol * 100,
		MinInvalid:        c.MinInvalid,
		MinCoherencePct:   c.MinCoherence * 100,
		PFDropThreshold:   c.PFDropThreshold,
		NightRatioSignal:  c.NightRatioSignal,
	}
}

var owners = map[domain.AnomalyType][2]string{
	domain.RealAnomaly:        {"Mantenimiento eléctrico", "Hoy"},
	domain.DataQuality:        {"Medición", "Esta semana"},
	domain.ExplainableAnomaly: {"Producción", "Esta semana"},
	domain.FalsePositive:      {"—", "Cerrado"},
}

// LatestReport builds the report of the latest completed analysis.
func (s *Service) LatestReport(ctx context.Context) (Report, error) {
	run, err := s.store.LatestRun(ctx)
	if err != nil {
		return Report{}, err
	}
	if run.Status != domain.RunCompleted {
		return Report{}, fmt.Errorf("%w: the latest analysis is %s", ErrConflict, run.Status)
	}
	sum, err := s.DashboardSummary(ctx)
	if err != nil {
		return Report{}, err
	}
	meters, err := s.ListMeters(ctx, MeterQuery{})
	if err != nil {
		return Report{}, err
	}
	as, err := s.Anomalies(ctx, AnomalyQuery{})
	if err != nil {
		return Report{}, err
	}
	r := Report{RunID: run.ID, Summary: sum, Methodology: methodologyOf(s.engine.Config()),
		Sources: []string{"readings.csv", "events.csv"}}
	if run.FinishedAt != nil {
		r.GeneratedAt = run.FinishedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if run.Summary != nil {
		r.Headline = run.Summary.Headline
	}
	for _, m := range meters {
		r.Contributions = append(r.Contributions, Contribution{MeterID: m.ID, Name: m.Name, DeltaKWh: round1(m.CurrentKWh - m.BaselineKWh)})
		if m.Status != domain.StatusOK {
			r.RuleFlags = append(r.RuleFlags, m)
		}
	}
	sort.Slice(r.Contributions, func(i, j int) bool { return r.Contributions[i].DeltaKWh > r.Contributions[j].DeltaKWh })
	for _, a := range as {
		d, err := s.Anomaly(ctx, a.ID)
		if err != nil {
			return Report{}, err
		}
		r.Findings = append(r.Findings, d)
		o := owners[a.Type]
		r.Plan = append(r.Plan, PlanItem{MeterID: a.MeterID, Type: a.Type, Action: a.RecommendedAction, Owner: o[0], Deadline: o[1], Steps: a.NextSteps})
	}
	return r, nil
}
