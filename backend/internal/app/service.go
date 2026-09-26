package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
	"github.com/yefersongallo/bia-energy/backend/internal/explain"
)

// Options configure the service.
type Options struct {
	StepDelay time.Duration // artificial pause between steps so the UI can show progress
	Clock     func() time.Time
	Logger    *slog.Logger
}

// Service implements every use case of the API.
type Service struct {
	store     Store
	engine    *analysis.Engine
	explainer explain.Explainer
	opts      Options

	mu      sync.Mutex
	running string // id of the run in progress, if any
	wg      sync.WaitGroup

	statsMu sync.Mutex
	stats   map[string]analysis.MeterStats

	markMu sync.Mutex
	marks  map[string]time.Time // end of the previous step, per run (step durations)

	// lifeMu serialises lifecycle changes: check-then-write must be atomic, or a
	// resolve and an investigate sent at the same time can reopen a resolved alarm.
	lifeMu sync.Mutex
}

// New builds the service.
func New(store Store, engine *analysis.Engine, explainer explain.Explainer, opts Options) *Service {
	if opts.Clock == nil {
		opts.Clock = func() time.Time { return time.Now().UTC() }
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Service{store: store, engine: engine, explainer: explainer, opts: opts}
}

// Wait blocks until background analyses finish (used by tests and shutdown).
func (s *Service) Wait() { s.wg.Wait() }

// statsByMeter computes (once) the rule-layer statistics of every meter.
func (s *Service) statsByMeter(ctx context.Context) (map[string]analysis.MeterStats, error) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	if s.stats != nil {
		return s.stats, nil
	}
	rs, err := s.store.Readings(ctx)
	if err != nil {
		return nil, err
	}
	grouped := map[string][]domain.Reading{}
	for _, r := range rs {
		grouped[r.MeterID] = append(grouped[r.MeterID], r)
	}
	out := map[string]analysis.MeterStats{}
	for id, list := range grouped {
		sort.Slice(list, func(i, j int) bool { return list[i].Timestamp.Before(list[j].Timestamp) })
		st := s.engine.ComputeStats(list)
		st.MeterID = id
		out[id] = st
	}
	s.stats = out
	return out, nil
}

// --- meters -----------------------------------------------------------------

// AnomalyRef is the compact view of an anomaly attached to a meter.
type AnomalyRef struct {
	ID         string               `json:"id"`
	Type       domain.AnomalyType   `json:"type"`
	Severity   domain.Severity      `json:"severity"`
	Rank       int                  `json:"rank"`
	Confidence float64              `json:"confidence"`
	Status     domain.AnomalyStatus `json:"status"`
}

// MeterSummary is one row of the meters table.
type MeterSummary struct {
	domain.Meter
	Status          domain.MeterStatus `json:"status"`
	StatusReason    string             `json:"status_reason"`
	PeriodKWh       float64            `json:"period_kwh"`
	BaselineKWh     float64            `json:"baseline_kwh"`
	CurrentKWh      float64            `json:"current_kwh"`
	VariationPct    float64            `json:"variation_pct"`
	InvalidReadings int                `json:"invalid_readings"`
	Daily           []float64          `json:"daily_kwh"`
	Anomaly         *AnomalyRef        `json:"anomaly"`
}

// MeterQuery filters and sorts the meters table.
type MeterQuery struct {
	Status string // "", OK, ALERT, CRITICAL
	Q      string // search by meter id or name
	Sort   string // severity (default) | consumption | variation
	Order  string // desc (default) | asc
}

func (s *Service) anomaliesByMeter(ctx context.Context) (map[string]domain.Anomaly, error) {
	as, err := s.store.Anomalies(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]domain.Anomaly{}
	for _, a := range as {
		out[a.MeterID] = a
	}
	return out, nil
}

func ref(a domain.Anomaly) *AnomalyRef {
	return &AnomalyRef{ID: a.ID, Type: a.Type, Severity: a.Severity, Rank: a.Rank, Confidence: a.Confidence, Status: a.Status}
}

func summarize(m domain.Meter, st analysis.MeterStats, a *domain.Anomaly) MeterSummary {
	daily := make([]float64, len(st.Days))
	for i, d := range st.Days {
		daily[i] = d.KWh
	}
	ms := MeterSummary{
		Meter: m, Status: st.Status, StatusReason: st.StatusReason, PeriodKWh: st.PeriodKWh, BaselineKWh: st.BaselineKWh, CurrentKWh: st.CurrentKWh,
		VariationPct: st.VariationPct, InvalidReadings: st.InvalidReadings, Daily: daily,
	}
	if a != nil {
		ms.Anomaly = ref(*a)
		ms.Status, ms.StatusReason = derivedStatus(*a, st)
	}
	return ms
}

// derivedStatus is the meter state once the AI verdict exists (plan, phase 2):
// OK for a false positive, CRITICAL for a HIGH real anomaly, ALERT otherwise.
// Before any analysis the rule status of the engine is shown.
func derivedStatus(a domain.Anomaly, st analysis.MeterStats) (domain.MeterStatus, string) {
	switch {
	case a.Type == domain.FalsePositive:
		return domain.StatusOK, "cambio explicado por un evento operativo y ya recuperado"
	case a.Type == domain.RealAnomaly && a.Severity == domain.SeverityHigh:
		return domain.StatusCritical, st.StatusReason
	default:
		return domain.StatusAlert, st.StatusReason
	}
}

var statusRank = map[domain.MeterStatus]int{domain.StatusCritical: 3, domain.StatusAlert: 2, domain.StatusOK: 1}

// ListMeters returns the meters table with filters, search and sorting.
func (s *Service) ListMeters(ctx context.Context, q MeterQuery) ([]MeterSummary, error) {
	meters, err := s.store.Meters(ctx)
	if err != nil {
		return nil, err
	}
	stats, err := s.statsByMeter(ctx)
	if err != nil {
		return nil, err
	}
	anoms, err := s.anomaliesByMeter(ctx)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	var out []MeterSummary
	for _, m := range meters {
		st := stats[m.ID]
		if needle != "" && !strings.Contains(strings.ToLower(m.ID), needle) && !strings.Contains(strings.ToLower(m.Name), needle) {
			continue
		}
		var ap *domain.Anomaly
		if a, ok := anoms[m.ID]; ok {
			ap = &a
		}
		ms := summarize(m, st, ap)
		if q.Status != "" && !strings.EqualFold(string(ms.Status), q.Status) {
			continue
		}
		out = append(out, ms)
	}
	severityScore := func(ms MeterSummary) float64 {
		score := float64(statusRank[ms.Status])
		if ms.Anomaly != nil {
			score += 10 + float64(10-ms.Anomaly.Rank)
		}
		return score
	}
	sort.SliceStable(out, func(i, j int) bool {
		switch q.Sort {
		case "consumption":
			return out[i].CurrentKWh > out[j].CurrentKWh
		case "variation":
			return math.Abs(out[i].VariationPct) > math.Abs(out[j].VariationPct)
		default:
			si, sj := severityScore(out[i]), severityScore(out[j])
			if si != sj {
				return si > sj
			}
			return abs(out[i].VariationPct) > abs(out[j].VariationPct)
		}
	})
	if q.Order == "asc" {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out, nil
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// MeterDetail is the full view of one meter.
type MeterDetail struct {
	MeterSummary
	Stats   analysis.MeterStats `json:"stats"`
	Events  []domain.Event      `json:"events"`
	Finding *domain.Anomaly     `json:"finding"`
}

// Meter returns the detail of a meter.
func (s *Service) Meter(ctx context.Context, id string) (MeterDetail, error) {
	meters, err := s.store.Meters(ctx)
	if err != nil {
		return MeterDetail{}, err
	}
	for _, m := range meters {
		if m.ID != id {
			continue
		}
		stats, err := s.statsByMeter(ctx)
		if err != nil {
			return MeterDetail{}, err
		}
		anoms, err := s.anomaliesByMeter(ctx)
		if err != nil {
			return MeterDetail{}, err
		}
		evs, err := s.MeterEvents(ctx, id)
		if err != nil {
			return MeterDetail{}, err
		}
		var ap *domain.Anomaly
		if a, ok := anoms[id]; ok {
			ap = &a
		}
		return MeterDetail{MeterSummary: summarize(m, stats[id], ap), Stats: stats[id], Events: evs, Finding: ap}, nil
	}
	return MeterDetail{}, fmt.Errorf("meter %s: %w", id, ErrNotFound)
}

// ReadingsQuery selects the resolution and an optional time window.
type ReadingsQuery struct {
	Bucket   string     // "" or hour: raw hourly readings; day: daily aggregates
	From, To *time.Time // inclusive bounds, both optional
}

// Readings returns raw hourly readings or daily aggregates of a meter.
func (s *Service) Readings(ctx context.Context, id string, q ReadingsQuery) (any, error) {
	if _, err := s.Meter(ctx, id); err != nil {
		return nil, err
	}
	in := func(t time.Time) bool {
		return (q.From == nil || !t.Before(*q.From)) && (q.To == nil || !t.After(*q.To))
	}
	if q.Bucket == "day" {
		stats, err := s.statsByMeter(ctx)
		if err != nil {
			return nil, err
		}
		out := []analysis.DayPoint{}
		for _, d := range stats[id].Days {
			// A day is kept when it overlaps the window.
			t, err := time.Parse("2006-01-02", d.Date)
			if err != nil || ((q.To == nil || !t.After(*q.To)) && (q.From == nil || t.Add(24*time.Hour).After(*q.From))) {
				out = append(out, d)
			}
		}
		return out, nil
	}
	rs, err := s.store.ReadingsByMeter(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []domain.Reading{}
	for _, r := range rs {
		if in(r.Timestamp) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Events returns every operational event, oldest first.
func (s *Service) Events(ctx context.Context) ([]domain.Event, error) {
	all, err := s.store.Events(ctx)
	if err != nil {
		return nil, err
	}
	out := append([]domain.Event{}, all...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out, nil
}

// MeterEvents returns the events of one meter.
func (s *Service) MeterEvents(ctx context.Context, id string) ([]domain.Event, error) {
	all, err := s.store.Events(ctx)
	if err != nil {
		return nil, err
	}
	out := []domain.Event{}
	for _, e := range all {
		if e.MeterID == id {
			out = append(out, e)
		}
	}
	return out, nil
}

// --- anomalies --------------------------------------------------------------

// AnomalyQuery filters the anomalies list.
type AnomalyQuery struct {
	Type     string
	Severity string
}

// Anomalies returns the findings of the latest completed run, by priority.
func (s *Service) Anomalies(ctx context.Context, q AnomalyQuery) ([]domain.Anomaly, error) {
	as, err := s.store.Anomalies(ctx)
	if err != nil {
		return nil, err
	}
	out := []domain.Anomaly{}
	for _, a := range as {
		if q.Type != "" && !strings.EqualFold(string(a.Type), q.Type) {
			continue
		}
		if q.Severity != "" && !strings.EqualFold(string(a.Severity), q.Severity) {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out, nil
}

// AnomalyDetail is the investigation view.
type AnomalyDetail struct {
	domain.Anomaly
	Meter          domain.Meter              `json:"meter"`
	Days           []analysis.DayPoint       `json:"days"`
	HourlyBaseline [24]float64               `json:"hourly_baseline"`
	HourlyCurrent  [24]float64               `json:"hourly_current"`
	IsAnomaly      bool                      `json:"anomaly"`
	Actions        []domain.AnomalyAction    `json:"actions"`
	KFactor        float64                   `json:"k_factor"`
	KMAD           float64                   `json:"k_mad"`
	Flagged        []analysis.FlaggedReading `json:"flagged_readings"`
}

// Anomaly returns the investigation view of one anomaly.
func (s *Service) Anomaly(ctx context.Context, id string) (AnomalyDetail, error) {
	a, err := s.store.Anomaly(ctx, id)
	if err != nil {
		return AnomalyDetail{}, err
	}
	md, err := s.Meter(ctx, a.MeterID)
	if err != nil {
		return AnomalyDetail{}, err
	}
	actions, err := s.store.Actions(ctx, a.ID)
	if err != nil {
		return AnomalyDetail{}, err
	}
	if actions == nil {
		actions = []domain.AnomalyAction{}
	}
	return AnomalyDetail{Anomaly: a, Meter: md.Meter, Days: md.Stats.Days, HourlyBaseline: md.Stats.HourlyBaseline, HourlyCurrent: md.Stats.HourlyCurrent,
		IsAnomaly: a.IsAnomaly(), Actions: actions, KFactor: md.Stats.KFactor, KMAD: md.Stats.KMAD, Flagged: md.Stats.Flagged}, nil
}

// UpdateAnomalyStatus moves an anomaly through its lifecycle.
func (s *Service) UpdateAnomalyStatus(ctx context.Context, id string, to domain.AnomalyStatus) (domain.Anomaly, error) {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	return s.updateStatus(ctx, id, to)
}

// updateStatus applies one transition; the caller holds lifeMu.
func (s *Service) updateStatus(ctx context.Context, id string, to domain.AnomalyStatus) (domain.Anomaly, error) {
	a, err := s.store.Anomaly(ctx, id)
	if err != nil {
		return domain.Anomaly{}, err
	}
	if !domain.CanTransition(a.Status, to) {
		return domain.Anomaly{}, fmt.Errorf("%w: %s → %s", ErrConflict, a.Status, to)
	}
	if err := s.store.UpdateAnomalyStatus(ctx, id, to); err != nil {
		return domain.Anomaly{}, err
	}
	a.Status = to
	return a, nil
}

// --- analysis runs ----------------------------------------------------------

// Steps of the pipeline, in the order the UI shows them.
var Steps = []domain.StepState{
	{Key: "readings", Label: "Lecturas"},
	{Key: "baseline", Label: "Baseline"},
	{Key: "detection", Label: "Detección"},
	{Key: "correlation", Label: "Correlación"},
	{Key: "events", Label: "Eventos"},
	{Key: "explanation", Label: "Explicación"},
	{Key: "recommendation", Label: "Recomendación"},
}

func newID(prefix string, t time.Time) string {
	b := make([]byte, 6) // unique even for many ids in the same minute
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s-%s", prefix, t.UTC().Format("0102-1504"), hex.EncodeToString(b))
}

// StartAnalysis launches the pipeline in the background and returns the run.
// Only one run executes at a time: a second call returns the running one.
func (s *Service) StartAnalysis(ctx context.Context) (domain.AnalysisRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != "" {
		return s.store.Run(ctx, s.running)
	}
	run := domain.AnalysisRun{ID: newID("A", s.opts.Clock()), Status: domain.RunRunning, StartedAt: s.opts.Clock(), Steps: make([]domain.StepState, len(Steps))}
	copy(run.Steps, Steps)
	for i := range run.Steps {
		run.Steps[i].Status = domain.RunPending
	}
	run.Steps[0].Status = domain.RunRunning
	if err := s.store.SaveRun(ctx, run); err != nil {
		return domain.AnalysisRun{}, err
	}
	s.running = run.ID
	s.wg.Add(1)
	work := run // the goroutine owns its own copy of the run and its steps
	work.Steps = append([]domain.StepState(nil), run.Steps...)
	go func(run domain.AnalysisRun) {
		defer s.wg.Done()
		bg, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.execute(bg, &run); err != nil {
			run.Status, run.Error = domain.RunFailed, err.Error()
			now := s.opts.Clock()
			run.FinishedAt = &now
			_ = s.store.SaveRun(bg, run)
			s.opts.Logger.Error("analysis failed", "run", run.ID, "err", err)
		}
		s.mu.Lock()
		s.running = ""
		s.mu.Unlock()
	}(work)
	return run, nil
}

func (s *Service) advance(ctx context.Context, run *domain.AnalysisRun, key, result string) {
	now := time.Now()
	s.markMu.Lock()
	if s.marks == nil {
		s.marks = map[string]time.Time{}
	}
	prev, ok := s.marks[run.ID]
	if !ok {
		prev = now
	}
	s.markMu.Unlock()
	for i := range run.Steps {
		if run.Steps[i].Key == key {
			run.Steps[i].Status, run.Steps[i].Result = domain.RunCompleted, result
			run.Steps[i].DurationMs = max(now.Sub(prev).Milliseconds(), 1)
			run.CurrentStep = i + 1
			if i+1 < len(run.Steps) {
				run.Steps[i+1].Status = domain.RunRunning
			}
		}
	}
	_ = s.store.SaveRun(ctx, *run)
	if s.opts.StepDelay > 0 {
		time.Sleep(s.opts.StepDelay)
	}
	s.markMu.Lock()
	if run.CurrentStep >= len(run.Steps) {
		delete(s.marks, run.ID)
	} else {
		s.marks[run.ID] = time.Now() // the pacing delay is not part of the next step
	}
	s.markMu.Unlock()
}

func (s *Service) execute(ctx context.Context, run *domain.AnalysisRun) error {
	s.markMu.Lock()
	if s.marks == nil {
		s.marks = map[string]time.Time{}
	}
	s.marks[run.ID] = time.Now()
	s.markMu.Unlock()
	meters, err := s.store.Meters(ctx)
	if err != nil {
		return err
	}
	readings, err := s.store.Readings(ctx)
	if err != nil {
		return err
	}
	events, err := s.store.Events(ctx)
	if err != nil {
		return err
	}
	res := s.engine.Analyze(analysis.Input{Meters: meters, Readings: readings, Events: events}, func(key, result string) {
		s.advance(ctx, run, key, result)
	})

	// explanation: the LLM writes words on top of the engine's evidence. The
	// findings are explained concurrently (one request each) to keep the run short.
	exps := make([]explain.Explanation, len(res.Findings))
	errs := make([]error, len(res.Findings))
	var wg sync.WaitGroup
	for i, f := range res.Findings {
		wg.Add(1)
		go func(i int, a domain.Anomaly) {
			defer wg.Done()
			exps[i], errs[i] = s.explainer.Explain(ctx, a)
		}(i, f.Anomaly)
	}
	wg.Wait()
	byLLM := 0
	anomalies := make([]domain.Anomaly, 0, len(res.Findings))
	for i, f := range res.Findings {
		a := f.Anomaly
		if errs[i] != nil {
			return fmt.Errorf("explain %s: %w", a.MeterID, errs[i])
		}
		exp := exps[i]
		if exp.Source == "claude" {
			byLLM++
		}
		a.ID = run.ID + "-" + a.MeterID
		a.AnalysisID = run.ID
		a.DetectedAt = s.opts.Clock()
		a.Reason, a.RecommendedAction, a.NextSteps, a.ExplainedBy = exp.Reason, exp.RecommendedAction, exp.NextSteps, exp.Source
		a.EvidenceSummary = exp.EvidenceSummary
		anomalies = append(anomalies, a)
	}
	s.advance(ctx, run, "explanation", fmt.Sprintf("%d explicaciones · %d con Claude", len(anomalies), byLLM))

	if err := s.store.ReplaceAnomalies(ctx, run.ID, anomalies); err != nil {
		return err
	}
	high, conf := 0, 0.0
	for _, a := range anomalies {
		if a.Severity == domain.SeverityHigh {
			high++
		}
		conf += a.Confidence
	}
	summary := &domain.RunSummary{Anomalies: len(anomalies), HighPriority: high}
	if len(anomalies) > 0 {
		summary.AvgConfidence = round2(conf / float64(len(anomalies)))
		summary.Headline = fmt.Sprintf("%d anomalías detectadas · %d requieren atención prioritaria", len(anomalies), high)
	} else {
		summary.Headline = "Sin anomalías: todos los medidores dentro de su baseline"
	}
	first := "—"
	if len(anomalies) > 0 {
		first = anomalies[0].MeterID
	}
	run.Summary = summary
	s.advance(ctx, run, "recommendation", fmt.Sprintf("%d de prioridad alta · %s primero", high, first))
	now := s.opts.Clock()
	run.Status, run.FinishedAt = domain.RunCompleted, &now
	return s.store.SaveRun(ctx, *run)
}

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }

// Run returns an analysis run by id ("latest" returns the most recent one).
func (s *Service) Run(ctx context.Context, id string) (domain.AnalysisRun, error) {
	if id == "latest" {
		return s.store.LatestRun(ctx)
	}
	return s.store.Run(ctx, id)
}

// --- dashboard --------------------------------------------------------------

// Summary is the dashboard KPI block.
type Summary struct {
	Meters         int                        `json:"meters"`
	StatusCounts   map[domain.MeterStatus]int `json:"status_counts"`
	PeriodKWh      float64                    `json:"period_kwh"`
	PeriodDays     int                        `json:"period_days"`
	CurrentKWh     float64                    `json:"current_kwh"`
	BaselineKWh    float64                    `json:"baseline_kwh"`
	VariationPct   float64                    `json:"variation_pct"`
	Anomalies      *int                       `json:"anomalies"`
	HighPriority   *int                       `json:"high_priority"`
	OpenAnomalies  *int                       `json:"open_anomalies"`
	AvgConfidence  *float64                   `json:"avg_confidence"`
	LastAnalysis   *domain.AnalysisRun        `json:"last_analysis"`
	InvalidReading int                        `json:"invalid_readings"`
	Readings       int                        `json:"readings"`
}

// DashboardSummary aggregates the KPIs.
func (s *Service) DashboardSummary(ctx context.Context) (Summary, error) {
	meters, err := s.store.Meters(ctx)
	if err != nil {
		return Summary{}, err
	}
	stats, err := s.statsByMeter(ctx)
	if err != nil {
		return Summary{}, err
	}
	anoms, err := s.anomaliesByMeter(ctx)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{Meters: len(meters), StatusCounts: map[domain.MeterStatus]int{domain.StatusOK: 0, domain.StatusAlert: 0, domain.StatusCritical: 0}}
	for _, m := range meters {
		st := stats[m.ID]
		status := st.Status
		if a, ok := anoms[m.ID]; ok {
			status, _ = derivedStatus(a, st)
		}
		out.StatusCounts[status]++
		out.CurrentKWh += st.CurrentKWh
		out.BaselineKWh += st.BaselineKWh
		out.InvalidReading += st.InvalidReadings
		out.Readings += st.Readings
		out.PeriodKWh += st.PeriodKWh
		out.PeriodDays = max(out.PeriodDays, len(st.Days))
	}
	out.CurrentKWh, out.BaselineKWh, out.PeriodKWh = round1(out.CurrentKWh), round1(out.BaselineKWh), round1(out.PeriodKWh)
	if out.BaselineKWh > 0 {
		out.VariationPct = round1((out.CurrentKWh/out.BaselineKWh - 1) * 100)
	}
	run, err := s.store.LatestRun(ctx)
	if err == nil {
		out.LastAnalysis = &run
	} else if !errors.Is(err, ErrNotFound) {
		return Summary{}, err
	}
	as, err := s.store.Anomalies(ctx)
	if err != nil {
		return Summary{}, err
	}
	if len(as) > 0 || (out.LastAnalysis != nil && out.LastAnalysis.Status == domain.RunCompleted) {
		n, high, open, conf := len(as), 0, 0, 0.0
		for _, a := range as {
			if a.Severity == domain.SeverityHigh {
				high++
			}
			if a.Status != domain.AnomalyResolved {
				open++
			}
			conf += a.Confidence
		}
		avg := 0.0
		if n > 0 {
			avg = round2(conf / float64(n))
		}
		out.Anomalies, out.HighPriority, out.OpenAnomalies, out.AvgConfidence = &n, &high, &open, &avg
	}
	return out, nil
}

func round1(v float64) float64 { return float64(int64(v*10+sign(v)*0.5)) / 10 }

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// RecoverInterrupted marks as failed a run left RUNNING by a previous process
// (a crash or a kill mid-analysis): nothing will ever finish it, and the UI would
// otherwise show it "in progress" forever. Call it once at start-up.
func (s *Service) RecoverInterrupted(ctx context.Context) error {
	run, err := s.store.LatestRun(ctx)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil || (run.Status != domain.RunRunning && run.Status != domain.RunPending) {
		return err
	}
	now := s.opts.Clock()
	run.Status, run.Error, run.FinishedAt = domain.RunFailed, "interrumpido: el servidor se reinició durante el análisis", &now
	for i := range run.Steps {
		if run.Steps[i].Status == domain.RunRunning || run.Steps[i].Status == domain.RunPending {
			run.Steps[i].Status = domain.RunFailed
		}
	}
	return s.store.SaveRun(ctx, run)
}
