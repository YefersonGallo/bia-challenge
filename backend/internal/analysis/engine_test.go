package analysis_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func challenge(t *testing.T) fixture {
	t.Helper()
	fs := fixtures(t)
	if len(fs) < 2 {
		t.Skip("challenge data not present")
	}
	return fs[1]
}

// M-112 must be found from the readings alone: removing every event changes nothing.
func TestM112DetectedWithoutEvents(t *testing.T) {
	for _, fx := range fixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			in := fx.input
			in.Events = nil
			f := findingFor(analysis.New(analysis.DefaultConfig()).Analyze(in, nil), "M-112")
			if f == nil || f.Anomaly.Type != domain.DataQuality || f.Anomaly.Severity != domain.SeverityHigh {
				t.Fatalf("M-112 without events = %+v", f)
			}
		})
	}
}

func TestChallengeChangePointsAndFlags(t *testing.T) {
	res := analyze(challenge(t))
	want := map[string]string{"M-109": "2026-09-12T14:00", "M-104": "2026-09-11T00:00", "M-106": "2026-09-08T00:00"}
	for id, at := range want {
		f := findingFor(res, id)
		if f.Anomaly.ChangePointAt == nil || f.Anomaly.ChangePointAt.Format("2006-01-02T15:04") != at {
			t.Errorf("%s change point = %v, want %s", id, f.Anomaly.ChangePointAt, at)
		}
	}
	if f := findingFor(res, "M-106"); f.Anomaly.EndedAt == nil || f.Anomaly.EndedAt.Format("15:04") != "12:00" {
		t.Errorf("M-106 should end at 12:00, got %v", f.Anomaly.EndedAt)
	}
	s := res.Stats["M-112"]
	flags := map[string]bool{}
	for _, fr := range s.Flagged {
		for _, fl := range fr.Flags {
			flags[fl] = true
		}
	}
	for _, fl := range []string{analysis.FlagVoltage, analysis.FlagJump, analysis.FlagPF} {
		if !flags[fl] {
			t.Errorf("M-112 missing flag %s", fl)
		}
	}
	if s.MaxFlagShare24h <= 0.10 {
		t.Errorf("M-112 flag share = %.2f, want > 0,10", s.MaxFlagShare24h)
	}
	for _, id := range []string{"M-101", "M-104", "M-106", "M-109"} {
		if n := len(res.Stats[id].Flagged); n > 2 {
			t.Errorf("%s has %d flagged readings, want ≤ 2", id, n)
		}
	}
}

func TestConfidenceBreakdownAndImpact(t *testing.T) {
	res := analyze(challenge(t))
	for _, f := range res.Findings {
		total := 0.0
		for _, c := range f.Anomaly.ConfidenceBreakdown {
			total += c.Weight
			if c.Score < 0 || c.Score > 1 {
				t.Errorf("%s %s score %.2f out of [0,1]", f.Anomaly.MeterID, c.Key, c.Score)
			}
		}
		if total < 0.999 || total > 1.001 {
			t.Errorf("%s weights sum %.3f", f.Anomaly.MeterID, total)
		}
	}
	im := findingFor(res, "M-109").Anomaly.Impact
	if im.ExtraKWhPerDay < 1100 || im.ExtraKWhPerDay > 1200 {
		t.Errorf("M-109 extra kWh/day = %.0f, want ≈ 1.150", im.ExtraKWhPerDay)
	}
	if im.ExtraKWhPerMonth < 33000 || im.ExtraKWhPerMonth > 36000 {
		t.Errorf("M-109 extra kWh/month = %.0f, want ≈ 34.000", im.ExtraKWhPerMonth)
	}
	if im.ReactiveRatio < 0.85 || im.ReactiveRatio > 0.95 {
		t.Errorf("M-109 reactive ratio = %.2f, want ≈ 0,91", im.ReactiveRatio)
	}
	if fp := findingFor(res, "M-106").Anomaly.Impact; fp.Normalized > 0.1 {
		t.Errorf("M-106 impact should be negligible, got %.2f", fp.Normalized)
	}
}

// An outage whose stated duration does not match the observed one does not explain it.
func TestOutageDurationMustMatch(t *testing.T) {
	fx := challenge(t)
	in := fx.input
	in.Events = append([]domain.Event(nil), fx.input.Events...)
	for i, e := range in.Events {
		if e.MeterID == "M-106" {
			in.Events[i].Description = "Scheduled maintenance outage for 48 hours"
		}
	}
	f := findingFor(analysis.New(analysis.DefaultConfig()).Analyze(in, nil), "M-106")
	if f.Anomaly.Evidence.EventExplainsShift {
		t.Fatal("a 48 h outage must not explain a 12 h drop")
	}
}

// Seen while it happens, an outage is shorter than announced: it still matches the
// event until it exceeds the announced duration.
func TestOngoingOutageMatchesItsEvent(t *testing.T) {
	fx := challenge(t)
	in := fx.input
	cut := time.Date(2026, 9, 8, 7, 0, 0, 0, time.UTC) // 8 h into the 12 h outage
	in.Readings = nil
	for _, r := range fx.input.Readings {
		if !r.Timestamp.After(cut) {
			in.Readings = append(in.Readings, r)
		}
	}
	f := findingFor(analysis.New(analysis.DefaultConfig()).Analyze(in, nil), "M-106")
	if f.Anomaly.Type != domain.ExplainableAnomaly || !f.Anomaly.Evidence.EventExplainsShift {
		t.Fatalf("ongoing outage = %s (explained %v): %s", f.Anomaly.Type, f.Anomaly.Evidence.EventExplainsShift, f.Anomaly.Reason)
	}
}

// The P1 must not depend on the order of the rows: M-109 and M-112 tie on the
// score, and the tie is broken by type (a real anomaly first), not by input order.
func TestPriorityDoesNotDependOnInputOrder(t *testing.T) {
	for _, f := range fixtures(t) {
		want := analyze(f)
		in := f.input
		in.Readings = append([]domain.Reading(nil), f.input.Readings...)
		in.Meters = append([]domain.Meter(nil), f.input.Meters...)
		for i, j := 0, len(in.Readings)-1; i < j; i, j = i+1, j-1 {
			in.Readings[i], in.Readings[j] = in.Readings[j], in.Readings[i]
		}
		for i, j := 0, len(in.Meters)-1; i < j; i, j = i+1, j-1 {
			in.Meters[i], in.Meters[j] = in.Meters[j], in.Meters[i]
		}
		got := analysis.New(analysis.DefaultConfig()).Analyze(in, nil)
		for i := range want.Findings {
			if got.Findings[i].Anomaly.MeterID != want.Findings[i].Anomaly.MeterID {
				t.Fatalf("%s: rank %d = %s with reversed input, %s otherwise", f.name, i+1, got.Findings[i].Anomaly.MeterID, want.Findings[i].Anomaly.MeterID)
			}
		}
		if got.Findings[0].Anomaly.MeterID != "M-109" {
			t.Fatalf("%s: P1 = %s", f.name, got.Findings[0].Anomaly.MeterID)
		}
	}
}

func TestDuplicatedFileDoesNotChangeTheResult(t *testing.T) {
	f := fixtures(t)[len(fixtures(t))-1]
	want := analyze(f)
	in := f.input
	in.Readings = append(append([]domain.Reading(nil), f.input.Readings...), f.input.Readings...)
	got := analysis.New(analysis.DefaultConfig()).Analyze(in, nil)
	if len(got.Findings) != len(want.Findings) {
		t.Fatalf("findings = %d, want %d", len(got.Findings), len(want.Findings))
	}
	for id, s := range got.Stats {
		if s.Status != want.Stats[id].Status || s.CurrentKWh != want.Stats[id].CurrentKWh {
			t.Fatalf("%s changed with a duplicated file: %s %.1f vs %s %.1f", id, s.Status, s.CurrentKWh, want.Stats[id].Status, want.Stats[id].CurrentKWh)
		}
	}
}

// A gap in the data is not a drop in consumption, and gaps never decide a
// verdict on their own (they must not hide a real change either).
func TestMissingHoursAreNotADrop(t *testing.T) {
	f := fixtures(t)[len(fixtures(t))-1]
	in := f.input
	last := f.input.Readings[len(f.input.Readings)-1].Timestamp
	first := f.input.Readings[0].Timestamp
	in.Readings = nil
	for _, r := range f.input.Readings {
		if r.MeterID == "M-101" && last.Sub(r.Timestamp) < 10*time.Hour && last.Sub(r.Timestamp) >= 2*time.Hour {
			continue // 8 hours missing on the last day
		}
		if r.MeterID == "M-109" && r.Timestamp.Sub(first) >= 30*time.Hour && r.Timestamp.Sub(first) < 36*time.Hour {
			continue // 6 hours missing in the baseline week of the real anomaly
		}
		in.Readings = append(in.Readings, r)
	}
	res := analysis.New(analysis.DefaultConfig()).Analyze(in, nil)
	s := res.Stats["M-101"]
	if s.MissingHours != 8 || math.Abs(s.VariationPct) > 10 || findingFor(res, "M-101") != nil {
		t.Fatalf("M-101: missing %d, variation %.1f%%, finding %v", s.MissingHours, s.VariationPct, findingFor(res, "M-101"))
	}
	if res.Findings[0].Anomaly.MeterID != "M-109" || res.Findings[0].Anomaly.Type != domain.RealAnomaly {
		t.Fatalf("gaps in the baseline week changed the P1: %s %s", res.Findings[0].Anomaly.MeterID, res.Findings[0].Anomaly.Type)
	}
}
