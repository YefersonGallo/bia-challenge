package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// --- heatmap -----------------------------------------------------------------

// HeatmapRow is one meter of the dashboard heatmap: deviation % per day.
type HeatmapRow struct {
	MeterID string              `json:"meter_id"`
	Name    string              `json:"name"`
	Status  domain.MeterStatus  `json:"status"`
	Anomaly *AnomalyRef         `json:"anomaly"`
	Days    []analysis.DayPoint `json:"days"`
}

// Heatmap returns the deviation of every meter against its baseline, day by day.
func (s *Service) Heatmap(ctx context.Context) ([]HeatmapRow, error) {
	ms, err := s.ListMeters(ctx, MeterQuery{})
	if err != nil {
		return nil, err
	}
	stats, err := s.statsByMeter(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]HeatmapRow, 0, len(ms))
	for _, m := range ms {
		out = append(out, HeatmapRow{MeterID: m.ID, Name: m.Name, Status: m.Status, Anomaly: m.Anomaly, Days: stats[m.ID].Days})
	}
	return out, nil
}

// --- baseline & forecast -------------------------------------------------------

// Baseline is the hour-of-day profile of a meter and its calibrated physical relation.
type Baseline struct {
	MeterID     string                    `json:"meter_id"`
	WindowDays  int                       `json:"window_days"`
	Median      [24]float64               `json:"median"`
	P10         [24]float64               `json:"p10"`
	P90         [24]float64               `json:"p90"`
	DailyKWh    float64                   `json:"daily_kwh"`
	KFactor     float64                   `json:"k_factor"`
	KMAD        float64                   `json:"k_mad"`
	KTolerance  float64                   `json:"k_tolerance"` // relative band used by the data-quality rule
	VoltageBand [2]float64                `json:"voltage_band"`
	Flagged     []analysis.FlaggedReading `json:"flagged_readings"`
}

// Baseline returns the profile used to judge a meter.
func (s *Service) Baseline(ctx context.Context, id string) (Baseline, error) {
	md, err := s.Meter(ctx, id)
	if err != nil {
		return Baseline{}, err
	}
	st, c := md.Stats, s.engine.Config()
	return Baseline{
		MeterID: id, WindowDays: c.BaselineDays, Median: st.HourlyBaseline, P10: st.HourlyP10, P90: st.HourlyP90,
		DailyKWh: st.BaselineKWh, KFactor: st.KFactor, KMAD: st.KMAD, KTolerance: c.CoherenceTol,
		VoltageBand: [2]float64{c.VoltageMin, c.VoltageMax}, Flagged: st.Flagged,
	}, nil
}

// ForecastPoint is one projected hour.
type ForecastPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Expected  float64   `json:"expected_kwh"`
	P10       float64   `json:"p10"`
	P90       float64   `json:"p90"`
	Projected float64   `json:"projected_kwh"`
}

// Forecast is the 24 h projection of a meter and the impact of its anomaly.
type Forecast struct {
	MeterID      string          `json:"meter_id"`
	Method       string          `json:"method"`
	LevelFactor  float64         `json:"level_factor"`
	Points       []ForecastPoint `json:"points"`
	ExpectedKWh  float64         `json:"expected_kwh"`
	ProjectedKWh float64         `json:"projected_kwh"`
	Impact       *domain.Impact  `json:"impact"`
}

// Forecast projects the next 24 hours with a seasonal-naive profile (median per
// hour of day), scaled by the observed level when a persistent change exists.
// It is a projection, not an ML prediction: 14 days of data do not support more.
func (s *Service) Forecast(ctx context.Context, id string) (Forecast, error) {
	md, err := s.Meter(ctx, id)
	if err != nil {
		return Forecast{}, err
	}
	rs, err := s.store.ReadingsByMeter(ctx, id)
	if err != nil {
		return Forecast{}, err
	}
	st := md.Stats
	f := Forecast{MeterID: id, LevelFactor: st.LevelFactor, Method: "perfil por hora del día (estacional ingenuo)"}
	if f.LevelFactor != 1 {
		f.Method += fmt.Sprintf(" × nivel observado desde el cambio (%s)", analysis.FormatNumber(f.LevelFactor, 2))
	}
	last := time.Now().UTC().Truncate(time.Hour)
	if len(rs) > 0 {
		last = rs[len(rs)-1].Timestamp
	}
	for i := 1; i <= 24; i++ {
		t := last.Add(time.Duration(i) * time.Hour)
		h := t.Hour()
		p := ForecastPoint{Timestamp: t, Expected: st.HourlyBaseline[h], P10: st.HourlyP10[h], P90: st.HourlyP90[h], Projected: round2(st.HourlyBaseline[h] * f.LevelFactor)}
		f.ExpectedKWh += p.Expected
		f.ProjectedKWh += p.Projected
		f.Points = append(f.Points, p)
	}
	f.ExpectedKWh, f.ProjectedKWh = round1(f.ExpectedKWh), round1(f.ProjectedKWh)
	if md.Finding != nil {
		f.Impact = md.Finding.Impact
	}
	return f, nil
}

// --- actions -------------------------------------------------------------------

// Operator actions and the lifecycle state each one moves the anomaly to.
var actionStatus = map[string]domain.AnomalyStatus{
	"acknowledge": domain.AnomalyAcknowledged,
	"investigate": domain.AnomalyInProgress,
	"validate":    domain.AnomalyInProgress,
	"resolve":     domain.AnomalyResolved,
	"dismiss":     domain.AnomalyResolved,
	"note":        "",
}

// ActionInput is an operator action with an optional note.
type ActionInput struct {
	Action string `json:"action"`
	Note   string `json:"note"`
}

// AddAction records an action on an anomaly and moves its lifecycle when the
// action implies it (acknowledging first when needed, e.g. "investigate" on a new one).
func (s *Service) AddAction(ctx context.Context, id, actor string, in ActionInput) (AnomalyDetail, error) {
	to, ok := actionStatus[strings.ToLower(in.Action)]
	if !ok {
		return AnomalyDetail{}, fmt.Errorf("%w: unknown action %q", ErrInvalid, in.Action)
	}
	if len(in.Note) > 2000 {
		return AnomalyDetail{}, fmt.Errorf("%w: note too long", ErrInvalid)
	}
	a, err := s.store.Anomaly(ctx, id)
	if err != nil {
		return AnomalyDetail{}, err
	}
	if to != "" && to != a.Status {
		if to == domain.AnomalyInProgress && a.Status == domain.AnomalyOpen {
			if _, err := s.UpdateAnomalyStatus(ctx, id, domain.AnomalyAcknowledged); err != nil {
				return AnomalyDetail{}, err
			}
		}
		if _, err := s.UpdateAnomalyStatus(ctx, id, to); err != nil {
			return AnomalyDetail{}, err
		}
	}
	act := domain.AnomalyAction{ID: newID("ACT", s.opts.Clock()), AnomalyID: id, Action: strings.ToLower(in.Action), Note: strings.TrimSpace(in.Note), Status: to, Actor: actor, At: s.opts.Clock()}
	if err := s.store.AddAction(ctx, act); err != nil {
		return AnomalyDetail{}, err
	}
	return s.Anomaly(ctx, id)
}
