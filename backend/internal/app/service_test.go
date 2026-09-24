package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
	"github.com/yefersongallo/bia-energy/backend/internal/explain"
	"github.com/yefersongallo/bia-energy/backend/internal/store/memory"
)

func newService(t *testing.T) *app.Service {
	t.Helper()
	st := memory.New()
	ds := dataset.Generate()
	if err := st.Seed(context.Background(), ds.Meters, ds.Readings, ds.Events); err != nil {
		t.Fatal(err)
	}
	return app.New(st, analysis.New(analysis.DefaultConfig()), explain.Template{}, app.Options{})
}

func analyze(t *testing.T, s *app.Service) domain.AnalysisRun {
	t.Helper()
	run, err := s.StartAnalysis(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.Wait()
	run, err = s.Run(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestAnalysisRunCompletesAllSteps(t *testing.T) {
	s := newService(t)
	run := analyze(t, s)
	if run.Status != domain.RunCompleted {
		t.Fatalf("status = %s (%s)", run.Status, run.Error)
	}
	for _, st := range run.Steps {
		if st.Status != domain.RunCompleted || st.Result == "" {
			t.Errorf("step %s = %s %q", st.Key, st.Status, st.Result)
		}
	}
	if run.Summary.Headline != "4 anomalías detectadas · 2 requieren atención prioritaria" {
		t.Errorf("headline = %q", run.Summary.Headline)
	}
}

func TestDashboardBeforeAndAfterAnalysis(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	before, err := s.DashboardSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Anomalies != nil || before.LastAnalysis != nil {
		t.Fatal("no anomalies must be reported before the first analysis")
	}
	if before.Meters != 12 || before.StatusCounts[domain.StatusOK] != 8 || before.StatusCounts[domain.StatusCritical] != 1 {
		t.Fatalf("unexpected counts: %+v", before.StatusCounts)
	}
	analyze(t, s)
	after, _ := s.DashboardSummary(ctx)
	if *after.Anomalies != 4 || *after.HighPriority != 2 {
		t.Fatalf("after: %d anomalies, %d high", *after.Anomalies, *after.HighPriority)
	}
}

func TestMetersFilterSearchAndSort(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	analyze(t, s)
	crit, _ := s.ListMeters(ctx, app.MeterQuery{Status: "CRITICAL"})
	if len(crit) != 1 || crit[0].ID != "M-109" {
		t.Fatalf("critical filter = %+v", crit)
	}
	found, _ := s.ListMeters(ctx, app.MeterQuery{Q: "m-11"})
	if len(found) != 3 { // M-110, M-111, M-112
		t.Fatalf("search returned %d meters", len(found))
	}
	bySeverity, _ := s.ListMeters(ctx, app.MeterQuery{})
	if bySeverity[0].ID != "M-109" || bySeverity[1].ID != "M-112" {
		t.Fatalf("severity order starts with %s, %s", bySeverity[0].ID, bySeverity[1].ID)
	}
	byConsumption, _ := s.ListMeters(ctx, app.MeterQuery{Sort: "consumption"})
	for i := 1; i < len(byConsumption); i++ {
		if byConsumption[i-1].CurrentKWh < byConsumption[i].CurrentKWh {
			t.Fatal("consumption sort is not descending")
		}
	}
}

func TestAnomalyLifecycle(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	analyze(t, s)
	as, _ := s.Anomalies(ctx, app.AnomalyQuery{})
	id := as[0].ID
	if _, err := s.UpdateAnomalyStatus(ctx, id, domain.AnomalyInProgress); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("OPEN → IN_PROGRESS must be rejected, got %v", err)
	}
	for _, to := range []domain.AnomalyStatus{domain.AnomalyAcknowledged, domain.AnomalyInProgress, domain.AnomalyResolved} {
		if _, err := s.UpdateAnomalyStatus(ctx, id, to); err != nil {
			t.Fatalf("→ %s: %v", to, err)
		}
	}
	a, _ := s.Anomaly(ctx, id)
	if a.Status != domain.AnomalyResolved {
		t.Fatalf("status = %s", a.Status)
	}
}

func TestAnomaliesFilterByType(t *testing.T) {
	s := newService(t)
	analyze(t, s)
	as, _ := s.Anomalies(context.Background(), app.AnomalyQuery{Type: "DATA_QUALITY"})
	if len(as) != 1 || as[0].MeterID != "M-112" {
		t.Fatalf("type filter = %+v", as)
	}
}

func TestReportRequiresCompletedAnalysis(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	if _, err := s.LatestReport(ctx); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected not found before any analysis, got %v", err)
	}
	analyze(t, s)
	r, err := s.LatestReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 4 || len(r.Plan) != 4 || r.Contributions[0].MeterID != "M-109" {
		t.Fatalf("report: %d findings, %d plan, first contribution %s", len(r.Findings), len(r.Plan), r.Contributions[0].MeterID)
	}
	if r.Plan[0].Owner != "Mantenimiento eléctrico" {
		t.Errorf("owner = %s", r.Plan[0].Owner)
	}
}

func TestOnlyOneRunAtATime(t *testing.T) {
	st := memory.New()
	ds := dataset.Generate()
	_ = st.Seed(context.Background(), ds.Meters, ds.Readings, ds.Events)
	s := app.New(st, analysis.New(analysis.DefaultConfig()), explain.Template{}, app.Options{StepDelay: 20 * time.Millisecond})
	a, _ := s.StartAnalysis(context.Background())
	b, _ := s.StartAnalysis(context.Background())
	s.Wait()
	if a.ID != b.ID {
		t.Fatalf("a second start must return the running analysis (%s vs %s)", a.ID, b.ID)
	}
}

func TestMeterNotFound(t *testing.T) {
	if _, err := newService(t).Meter(context.Background(), "M-999"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
