package analysis_test

import (
	"testing"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

func run(t *testing.T) analysis.Result {
	t.Helper()
	ds := dataset.Generate()
	return analysis.New(analysis.DefaultConfig()).Analyze(analysis.Input{Meters: ds.Meters, Readings: ds.Readings, Events: ds.Events}, nil)
}

func findingFor(res analysis.Result, id string) *analysis.Finding {
	for i := range res.Findings {
		if res.Findings[i].Anomaly.MeterID == id {
			return &res.Findings[i]
		}
	}
	return nil
}

// The four scenarios of the challenge (section 9) must be classified exactly.
func TestExpectedScenarios(t *testing.T) {
	res := run(t)
	cases := []struct {
		meter    string
		typ      domain.AnomalyType
		severity domain.Severity
	}{
		{"M-109", domain.RealAnomaly, domain.SeverityHigh},
		{"M-112", domain.DataQuality, domain.SeverityHigh},
		{"M-104", domain.ExplainableAnomaly, domain.SeverityMedium},
		{"M-106", domain.FalsePositive, domain.SeverityLow},
	}
	for _, c := range cases {
		t.Run(c.meter, func(t *testing.T) {
			f := findingFor(res, c.meter)
			if f == nil {
				t.Fatalf("expected a finding for %s", c.meter)
			}
			if f.Anomaly.Type != c.typ {
				t.Errorf("type = %s, want %s", f.Anomaly.Type, c.typ)
			}
			if f.Anomaly.Severity != c.severity {
				t.Errorf("severity = %s, want %s", f.Anomaly.Severity, c.severity)
			}
			if len(f.Anomaly.Evidence.Signals) == 0 {
				t.Error("finding without evidence signals")
			}
		})
	}
}

// Normal meters must not raise false alarms.
func TestNormalMetersProduceNoFindings(t *testing.T) {
	res := run(t)
	if len(res.Findings) != 4 {
		ids := []string{}
		for _, f := range res.Findings {
			ids = append(ids, f.Anomaly.MeterID)
		}
		t.Fatalf("expected exactly 4 findings, got %d: %v", len(res.Findings), ids)
	}
}

// M-109 must be first: it is the only large, sustained, unexplained change.
func TestPrioritization(t *testing.T) {
	res := run(t)
	order := []string{}
	for _, f := range res.Findings {
		order = append(order, f.Anomaly.MeterID)
	}
	want := []string{"M-109", "M-112", "M-104", "M-106"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("priority order = %v, want %v", order, want)
		}
	}
	if res.Findings[0].Anomaly.Rank != 1 {
		t.Errorf("M-109 rank = %d", res.Findings[0].Anomaly.Rank)
	}
}

func TestM109Evidence(t *testing.T) {
	res := run(t)
	f := findingFor(res, "M-109")
	ev := f.Anomaly.Evidence
	if ev.VariationPct < 95 || ev.VariationPct > 112 {
		t.Errorf("variation = %.1f%%, want ≈ +103,7%%", ev.VariationPct)
	}
	if ev.OnsetDay != 8 {
		t.Errorf("onset day = %d, want 8", ev.OnsetDay)
	}
	if ev.EventExplainsShift || len(ev.RelatedEvents) != 0 {
		t.Error("M-109 must not be explained by any event")
	}
	codes := map[string]bool{}
	for _, s := range ev.Signals {
		codes[s.Code] = true
	}
	for _, c := range []string{"PERSISTENT_SHIFT", "NO_EVENT", "PF_DROP", "NIGHT_PATTERN"} {
		if !codes[c] {
			t.Errorf("missing signal %s", c)
		}
	}
	if f.Anomaly.Confidence < 0.85 {
		t.Errorf("confidence = %.2f, want ≥ 0,85", f.Anomaly.Confidence)
	}
	if f.Stats.Status != domain.StatusCritical {
		t.Errorf("rule status = %s, want CRITICAL", f.Stats.Status)
	}
}

func TestM112IsDataQualityDespiteStableConsumption(t *testing.T) {
	res := run(t)
	s := res.Stats["M-112"]
	if s.InvalidReadings != 25 {
		t.Errorf("invalid readings = %d, want 25 (19 PF>1 + 6 zero voltage)", s.InvalidReadings)
	}
	if s.VariationPct > 5 || s.VariationPct < -5 {
		t.Errorf("M-112 consumption should be stable, got %.1f%%", s.VariationPct)
	}
	if s.Coherence >= 0.9 {
		t.Errorf("coherence = %.2f, want < 0,90", s.Coherence)
	}
}

func TestM106ShutdownIsRecoveredAndExplained(t *testing.T) {
	f := findingFor(run(t), "M-106")
	ev := f.Anomaly.Evidence
	if !ev.EventExplainsShift || len(ev.RelatedEvents) != 1 {
		t.Fatalf("M-106 should be explained by the scheduled shutdown")
	}
	if ev.OnsetDay != 10 || ev.EndDay != 12 {
		t.Errorf("episode days %d–%d, want 10–12", ev.OnsetDay, ev.EndDay)
	}
	if f.Anomaly.IsAnomaly() {
		t.Error("a false positive must have anomaly=false")
	}
}

func TestPipelineReportsSteps(t *testing.T) {
	ds := dataset.Generate()
	var steps []string
	analysis.New(analysis.DefaultConfig()).Analyze(analysis.Input{Meters: ds.Meters, Readings: ds.Readings, Events: ds.Events}, func(k, _ string) { steps = append(steps, k) })
	want := []string{"readings", "baseline", "detection", "correlation", "events"}
	if len(steps) != len(want) {
		t.Fatalf("steps = %v", steps)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Fatalf("steps = %v, want %v", steps, want)
		}
	}
}

func TestEmptyMeter(t *testing.T) {
	s := analysis.New(analysis.DefaultConfig()).ComputeStats(nil)
	if s.Status != domain.StatusOK {
		t.Errorf("empty meter status = %s", s.Status)
	}
}
