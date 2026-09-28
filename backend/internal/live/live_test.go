package live_test

import (
	"context"
	"testing"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
	"github.com/yefersongallo/bia-energy/backend/internal/ingest"
	"github.com/yefersongallo/bia-energy/backend/internal/live"
)

type fixture struct {
	name     string
	meters   []domain.Meter
	readings []domain.Reading
	events   []domain.Event
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	d, err := ingest.LoadDir("../../data")
	if err != nil {
		t.Fatal(err)
	}
	s := dataset.Generate()
	return []fixture{{"challenge", d.Meters, d.Readings, d.Events}, {"synthetic", s.Meters, s.Readings, s.Events}}
}

func replay(t *testing.T, f fixture) (*live.Runner, *live.Hub, []live.AlertChange) {
	t.Helper()
	hub := live.NewHub(100_000)
	ch, _, _, cancel := hub.Subscribe(0)
	defer cancel()
	r := live.NewRunner(analysis.New(analysis.DefaultConfig()), hub, f.meters, f.readings, f.events, live.Options{})
	for r.Step() {
	}
	var changes []live.AlertChange
	for {
		select {
		case m := <-ch:
			if c, ok := m.Data.(live.AlertChange); ok {
				changes = append(changes, c)
			}
		default:
			return r, hub, changes
		}
	}
}

// Streaming must end where the batch analysis ends: same meters, same type and
// severity; a false positive ends as a closed alert.
func TestStreamingMatchesBatch(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			batch := analysis.New(analysis.DefaultConfig()).Analyze(analysis.Input{Meters: f.meters, Readings: f.readings, Events: f.events}, nil)
			r, _, changes := replay(t, f)

			final := map[string]live.Alert{}
			for _, a := range r.Alerts() {
				if _, seen := final[a.MeterID]; !seen { // open alerts first, then newest closed
					final[a.MeterID] = a
				}
			}
			for _, fd := range batch.Findings {
				a, ok := final[fd.Anomaly.MeterID]
				if !ok {
					t.Fatalf("%s: batch finding %s missing in the stream", fd.Anomaly.MeterID, fd.Anomaly.Type)
				}
				if fd.Anomaly.Type == domain.FalsePositive {
					if a.State != live.AlertClosed || a.Type != domain.FalsePositive {
						t.Fatalf("%s: false positive must end closed, got %s %s", a.MeterID, a.State, a.Type)
					}
					continue
				}
				if a.State != live.AlertConfirmed || a.Type != fd.Anomaly.Type || a.Severity != fd.Anomaly.Severity {
					t.Fatalf("%s: stream %s %s %s, batch %s %s", a.MeterID, a.State, a.Type, a.Severity, fd.Anomaly.Type, fd.Anomaly.Severity)
				}
			}
			// No meter outside the batch findings may ever reach CONFIRMED.
			want := map[string]bool{}
			for _, fd := range batch.Findings {
				want[fd.Anomaly.MeterID] = true
			}
			for _, c := range changes {
				if c.Change == "confirmed" && !want[c.Alert.MeterID] {
					t.Fatalf("%s confirmed in the stream but normal in batch", c.Alert.MeterID)
				}
			}
		})
	}
}

func TestM109IsConfirmedHoursAfterTheChange(t *testing.T) {
	_, _, changes := replay(t, fixtures(t)[0])
	var cand, conf *live.Alert
	for i, c := range changes {
		if c.Alert.MeterID != "M-109" {
			continue
		}
		if c.Change == "candidate" && cand == nil {
			cand = &changes[i].Alert
		}
		if c.Change == "confirmed" && conf == nil {
			conf = &changes[i].Alert
		}
	}
	if cand == nil || conf == nil {
		t.Fatal("M-109 must go candidate → confirmed")
	}
	change := time.Date(2026, 9, 12, 14, 0, 0, 0, time.UTC)
	if cand.OpenedAt.Before(change) || cand.OpenedAt.Sub(change) > 8*time.Hour {
		t.Fatalf("candidate at %s, change at %s", cand.OpenedAt, change)
	}
	if got := conf.ConfirmedAt.Sub(cand.OpenedAt); got < 3*time.Hour {
		t.Fatalf("confirmed %s after the candidate, want ≥ 3h", got)
	}
}

func TestM106OutageClosesAsExplained(t *testing.T) {
	_, _, changes := replay(t, fixtures(t)[0])
	for _, c := range changes {
		if c.Alert.MeterID == "M-106" && c.Alert.Type == domain.RealAnomaly {
			t.Fatalf("the scheduled outage must never look like a real anomaly: %+v", c)
		}
		if c.Alert.MeterID == "M-106" && c.Change == "closed" {
			if c.Alert.Type != domain.FalsePositive {
				t.Fatalf("M-106 closed as %s", c.Alert.Type)
			}
			return
		}
	}
	t.Fatal("M-106 alert was never closed")
}

func TestPreloadStartsOnDay7(t *testing.T) {
	f := fixtures(t)[0]
	r := live.NewRunner(analysis.New(analysis.DefaultConfig()), live.NewHub(0), f.meters, f.readings, f.events, live.Options{})
	st := r.State()
	if st.Start != time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC) || st.TotalHours != 8*24 || st.Hour != 0 {
		t.Fatalf("state = %+v", st)
	}
	if len(r.Alerts()) != 0 {
		t.Fatalf("no alerts expected before streaming, got %+v", r.Alerts())
	}
}

func TestHubResumesFromLastEventID(t *testing.T) {
	h := live.NewHub(3)
	for i := 0; i < 5; i++ {
		h.Publish("tick", i)
	}
	_, missed, resumed, cancel := h.Subscribe(3)
	cancel()
	if !resumed || len(missed) != 2 || missed[0].ID != 4 {
		t.Fatalf("resume from 3: resumed=%v missed=%+v", resumed, missed)
	}
	if _, _, resumed, cancel := h.Subscribe(1); resumed {
		cancel()
		t.Fatal("id 1 fell out of the buffer: a snapshot is required")
	} else {
		cancel()
	}
}

func TestHubDropsSlowSubscribers(t *testing.T) {
	h := live.NewHub(10)
	ch, _, _, cancel := h.Subscribe(0)
	defer cancel()
	for i := 0; i < 300; i++ {
		h.Publish("tick", i)
	}
	n := 0
	for range ch { // closed once the buffer overflowed
		n++
	}
	if n == 0 || h.Subscribers() != 0 {
		t.Fatalf("received %d, subscribers %d", n, h.Subscribers())
	}
}

func TestControlsAndLoop(t *testing.T) {
	f := fixtures(t)[0]
	hub := live.NewHub(0)
	r := live.NewRunner(analysis.New(analysis.DefaultConfig()), hub, f.meters, f.readings, f.events, live.Options{Step: 5 * time.Millisecond})
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go r.Loop(ctx)
	if _, ok := r.Control("speed", 100); ok {
		t.Fatal("speed 100 must be rejected")
	}
	r.Control(live.ActionSpeed, 4)
	r.Control(live.ActionStart, 0)
	deadline := time.Now().Add(3 * time.Second)
	for r.State().Hour < 5 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	st, _ := r.Control(live.ActionPause, 0)
	if st.Hour < 5 || st.Running {
		t.Fatalf("after start/pause: %+v", st)
	}
	time.Sleep(30 * time.Millisecond)
	if r.State().Hour != st.Hour {
		t.Fatal("clock advanced while paused")
	}
	if st, _ = r.Control(live.ActionReset, 0); st.Hour != 0 {
		t.Fatalf("reset: %+v", st)
	}
}

func TestHubCloseEndsStreamsAndOldIdsGetASnapshot(t *testing.T) {
	h := live.NewHubFrom(8, 5_000_000)
	for i := 0; i < 3; i++ {
		h.Publish("tick", i)
	}
	if _, _, resumed, cancel := h.Subscribe(42); resumed { // an id from a previous process
		cancel()
		t.Fatal("an id older than the process must get a snapshot")
	} else {
		cancel()
	}
	ch, _, _, _ := h.Subscribe(0)
	h.Close()
	if _, ok := <-ch; ok {
		t.Fatal("Close must end open subscriptions")
	}
	late, _, _, _ := h.Subscribe(0)
	if _, ok := <-late; ok {
		t.Fatal("a closed hub accepts no new subscriptions")
	}
}

// The real speed follows the nominal one: the work of each tick is not added to the wait.
func TestReplaySpeedMatchesTheNominalOne(t *testing.T) {
	f := fixtures(t)[0]
	r := live.NewRunner(analysis.New(analysis.DefaultConfig()), live.NewHub(0), f.meters, f.readings, f.events, live.Options{Step: 400 * time.Millisecond})
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go r.Loop(ctx)
	r.Control(live.ActionSpeed, 4) // 100 ms per simulated hour
	start := time.Now()
	r.Control(live.ActionStart, 0)
	for r.State().Hour < 8 && time.Since(start) < 5*time.Second {
		time.Sleep(2 * time.Millisecond)
	}
	r.Control(live.ActionPause, 0)
	elapsed := time.Since(start)
	// Nominal: 8 × 100 ms = 800 ms. When the analysis time of each tick was added
	// to the wait, the replay ran ~15–25 % slower than announced.
	if elapsed > 950*time.Millisecond {
		t.Fatalf("8 simulated hours took %s at 4× (nominal 800 ms)", elapsed)
	}
}

// A CSV import reloads the replay: it restarts with the new hours and every client
// gets a snapshot.
func TestReloadRestartsWithTheNewReadings(t *testing.T) {
	f := fixtures(t)[0]
	hub := live.NewHub(100)
	r := live.NewRunner(analysis.New(analysis.DefaultConfig()), hub, f.meters, f.readings, f.events, live.Options{})
	for i := 0; i < 10; i++ {
		r.Step()
	}
	before := r.State()
	extra := f.readings[len(f.readings)-1]
	extra.Timestamp = extra.Timestamp.Add(time.Hour)
	ch, _, _, cancel := hub.Subscribe(0)
	defer cancel()

	r.Reload(append(append([]domain.Reading(nil), f.readings...), extra))
	st := r.State()
	if st.Hour != 0 || st.TotalHours != before.TotalHours+1 {
		t.Fatalf("after reload: %+v (before %+v)", st, before)
	}
	if m := <-ch; m.Kind != "snapshot" {
		t.Fatalf("first message after reload = %s, want snapshot", m.Kind)
	}
}
