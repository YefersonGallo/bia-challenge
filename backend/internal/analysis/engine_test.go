package analysis_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
	"github.com/yefersongallo/bia-energy/backend/internal/ingest"
)

// The engine is checked against two datasets with different shapes:
//
//   - challenge: the official readings.csv / events.csv of the technical test
//     (changes start on days 8, 11 and 12; M-106 is a 12 h outage; M-112 has
//     erratic voltage and power factor while its consumption is stable).
//   - synthetic: the deterministic generator (changes start on day 8; M-106 is a
//     72 h shutdown; M-112 has PF > 1 and 0 V readings).
//
// Both must produce exactly the four expected verdicts in the same order.
type fixture struct {
	name     string
	input    analysis.Input
	onset    map[string]int // expected onset day of each episode
	shutdown [2]int         // expected M-106 episode days
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	ds := dataset.Generate()
	fs := []fixture{{
		name:     "synthetic",
		input:    analysis.Input{Meters: ds.Meters, Readings: ds.Readings, Events: ds.Events},
		onset:    map[string]int{"M-109": 8, "M-104": 8},
		shutdown: [2]int{10, 12},
	}}
	dir := filepath.Join("..", "..", "data")
	if _, err := os.Stat(filepath.Join(dir, "readings.csv")); err == nil {
		d, err := ingest.LoadDir(dir)
		if err != nil {
			t.Fatalf("load challenge data: %v", err)
		}
		fs = append(fs, fixture{
			name:     "challenge",
			input:    analysis.Input{Meters: d.Meters, Readings: d.Readings, Events: d.Events},
			onset:    map[string]int{"M-109": 12, "M-104": 11},
			shutdown: [2]int{8, 8},
		})
	}
	return fs
}

func analyze(f fixture) analysis.Result {
	return analysis.New(analysis.DefaultConfig()).Analyze(f.input, nil)
}

func findingFor(res analysis.Result, id string) *analysis.Finding {
	for i := range res.Findings {
		if res.Findings[i].Anomaly.MeterID == id {
			return &res.Findings[i]
		}
	}
	return nil
}

func signalCodes(f *analysis.Finding) map[string]bool {
	codes := map[string]bool{}
	for _, s := range f.Anomaly.Evidence.Signals {
		codes[s.Code] = true
	}
	return codes
}

// The four scenarios of the challenge (section 9) must be classified exactly,
// with M-109 first, and no other meter may raise an alarm.
func TestExpectedResults(t *testing.T) {
	want := []struct {
		meter    string
		typ      domain.AnomalyType
		severity domain.Severity
	}{
		{"M-109", domain.RealAnomaly, domain.SeverityHigh},
		{"M-112", domain.DataQuality, domain.SeverityHigh},
		{"M-104", domain.ExplainableAnomaly, domain.SeverityMedium},
		{"M-106", domain.FalsePositive, domain.SeverityLow},
	}
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			res := analyze(fx)
			if len(res.Findings) != len(want) {
				ids := []string{}
				for _, f := range res.Findings {
					ids = append(ids, f.Anomaly.MeterID)
				}
				t.Fatalf("expected %d findings, got %v", len(want), ids)
			}
			for i, w := range want {
				f := res.Findings[i]
				if f.Anomaly.MeterID != w.meter || f.Anomaly.Type != w.typ || f.Anomaly.Severity != w.severity {
					t.Errorf("#%d = %s %s %s, want %s %s %s", i+1, f.Anomaly.MeterID, f.Anomaly.Type, f.Anomaly.Severity, w.meter, w.typ, w.severity)
				}
				if f.Anomaly.Rank != i+1 {
					t.Errorf("%s rank = %d, want %d", w.meter, f.Anomaly.Rank, i+1)
				}
				if len(f.Anomaly.Evidence.Signals) == 0 {
					t.Errorf("%s without evidence signals", w.meter)
				}
			}
		})
	}
}

func TestM109IsARealAnomaly(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			res := analyze(fx)
			f := findingFor(res, "M-109")
			ev := f.Anomaly.Evidence
			if ev.VariationPct < 95 || ev.VariationPct > 115 {
				t.Errorf("variation = %.1f%%, want ≈ +100%%", ev.VariationPct)
			}
			if ev.OnsetDay != fx.onset["M-109"] {
				t.Errorf("onset day = %d, want %d", ev.OnsetDay, fx.onset["M-109"])
			}
			if ev.EventExplainsShift {
				t.Error("M-109 must not be explained by any event")
			}
			codes := signalCodes(f)
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
			// A regime change is not a data problem, even though kWh / (V·I·PF) moved.
			if f.Stats.QualityIssue(analysis.DefaultConfig()) {
				t.Errorf("M-109 flagged as data quality: %d suspicious readings", f.Stats.InvalidReadings)
			}
		})
	}
}

func TestM112IsDataQualityDespiteStableConsumption(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			res := analyze(fx)
			s := res.Stats["M-112"]
			if s.InvalidReadings < analysis.DefaultConfig().MinInvalid {
				t.Errorf("suspicious readings = %d", s.InvalidReadings)
			}
			if s.Episode != nil {
				t.Errorf("M-112 consumption should not shift, got episode %+v", *s.Episode)
			}
			if s.Status != domain.StatusAlert {
				t.Errorf("status = %s, want ALERT", s.Status)
			}
		})
	}
}

func TestChallengeM112Signals(t *testing.T) {
	fs := fixtures(t)
	if len(fs) < 2 {
		t.Skip("challenge data not present")
	}
	f := findingFor(analyze(fs[1]), "M-112")
	codes := signalCodes(f)
	for _, c := range []string{"VOLTAGE_JUMPS", "PF_JUMPS", "STABLE_CONSUMPTION", "DQ_EVENT"} {
		if !codes[c] {
			t.Errorf("missing signal %s", c)
		}
	}
	if f.Anomaly.Evidence.OnsetDay != 13 {
		t.Errorf("issues start day = %d, want 13", f.Anomaly.Evidence.OnsetDay)
	}
}

func TestM104IsExplainedByTheNewLine(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			f := findingFor(analyze(fx), "M-104")
			ev := f.Anomaly.Evidence
			if !ev.EventExplainsShift || len(ev.RelatedEvents) != 1 {
				t.Fatal("M-104 should be explained by the production line event")
			}
			if ev.OnsetDay != fx.onset["M-104"] {
				t.Errorf("onset day = %d, want %d", ev.OnsetDay, fx.onset["M-104"])
			}
			if ev.VariationPct < 40 || ev.VariationPct > 55 {
				t.Errorf("variation = %.1f%%, want ≈ +47%%", ev.VariationPct)
			}
		})
	}
}

func TestM106ShutdownIsRecoveredAndExplained(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			f := findingFor(analyze(fx), "M-106")
			ev := f.Anomaly.Evidence
			if !ev.EventExplainsShift || len(ev.RelatedEvents) != 1 {
				t.Fatalf("M-106 should be explained by the scheduled shutdown")
			}
			if ev.OnsetDay != fx.shutdown[0] || ev.EndDay != fx.shutdown[1] {
				t.Errorf("episode days %d–%d, want %v", ev.OnsetDay, ev.EndDay, fx.shutdown)
			}
			if ev.ShiftPct > -50 {
				t.Errorf("shift = %.1f%%, want a deep drop", ev.ShiftPct)
			}
			if f.Anomaly.IsAnomaly() {
				t.Error("a false positive must have anomaly=false")
			}
		})
	}
}

// An UNKNOWN event ("no operational event reported") never explains a change.
func TestUnknownEventDoesNotExplain(t *testing.T) {
	ds := dataset.Generate()
	events := append(ds.Events, domain.Event{ID: "EV-X", MeterID: "M-109", Timestamp: dataset.Start.Add(7 * 24 * 60 * 60 * 1e9), Type: "UNKNOWN", Description: "No operational event reported"})
	res := analysis.New(analysis.DefaultConfig()).Analyze(analysis.Input{Meters: ds.Meters, Readings: ds.Readings, Events: events}, nil)
	f := findingFor(res, "M-109")
	if f.Anomaly.Type != domain.RealAnomaly || len(f.Anomaly.Evidence.RelatedEvents) != 1 {
		t.Fatalf("M-109 = %s with %d related events", f.Anomaly.Type, len(f.Anomaly.Evidence.RelatedEvents))
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
