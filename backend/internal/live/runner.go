package live

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Options of the replay.
type Options struct {
	StartDay     int           // first streamed day; earlier days are preloaded (default 7)
	Step         time.Duration // wall time per simulated hour at speed 1 (default 500 ms)
	ConfirmAfter time.Duration // simulated time a candidate must persist (default 3 h)
	Recent       int           // readings per meter kept in the snapshot (default 48)
}

// Point is one hourly reading as streamed to the browser.
type Point struct {
	MeterID     string    `json:"meter_id"`
	Timestamp   time.Time `json:"timestamp"`
	KWh         float64   `json:"consumption_kwh"`
	VoltageV    float64   `json:"voltage_v"`
	CurrentA    float64   `json:"current_a"`
	PowerFactor float64   `json:"power_factor"`
}

// MeterLive is the live state of one meter.
type MeterLive struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	Status domain.MeterStatus `json:"status"`
	Recent []Point            `json:"recent"`
}

// State is the clock and the controls.
type State struct {
	Clock      time.Time `json:"clock"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Hour       int       `json:"hour"`        // streamed hours so far
	TotalHours int       `json:"total_hours"` // hours to stream
	Running    bool      `json:"running"`
	Finished   bool      `json:"finished"`
	Speed      float64   `json:"speed"`
	StepMs     int64     `json:"step_ms"` // wall ms per simulated hour at the current speed
	LastTickMs float64   `json:"last_tick_ms"`
}

// Snapshot is the first message of every (non-resumed) connection.
type Snapshot struct {
	State  State       `json:"state"`
	Meters []MeterLive `json:"meters"`
	Alerts []Alert     `json:"alerts"`
}

// Tick is published once per simulated hour.
type Tick struct {
	State    State                         `json:"state"`
	Readings []Point                       `json:"readings"`
	Status   map[string]domain.MeterStatus `json:"status"`
}

// Runner replays the dataset on a simulated clock.
type Runner struct {
	engine *analysis.Engine
	hub    *Hub
	opts   Options

	meters []domain.Meter
	events []domain.Event
	hours  []time.Time // distinct hours, ascending
	byHour map[time.Time][]domain.Reading

	mu       sync.Mutex
	mgr      *Manager
	window   []domain.Reading
	next     int // index in hours of the next hour to stream
	first    int // index of the first streamed hour
	running  bool
	speed    float64
	lastTick time.Duration
	wake     chan struct{}
}

// NewRunner prepares the replay; call Loop to drive it in real time or Step to advance manually.
func NewRunner(engine *analysis.Engine, hub *Hub, meters []domain.Meter, readings []domain.Reading, events []domain.Event, opts Options) *Runner {
	if opts.StartDay <= 0 {
		opts.StartDay = 7
	}
	if opts.Step <= 0 {
		opts.Step = 500 * time.Millisecond
	}
	if opts.ConfirmAfter <= 0 {
		opts.ConfirmAfter = 3 * time.Hour
	}
	if opts.Recent <= 0 {
		opts.Recent = 48
	}
	r := &Runner{engine: engine, hub: hub, opts: opts, meters: meters, events: events, byHour: map[time.Time][]domain.Reading{}, speed: 1, wake: make(chan struct{}, 1)}
	for _, x := range readings {
		h := x.Timestamp.UTC().Truncate(time.Hour)
		if _, ok := r.byHour[h]; !ok {
			r.hours = append(r.hours, h)
		}
		r.byHour[h] = append(r.byHour[h], x)
	}
	sort.Slice(r.hours, func(i, j int) bool { return r.hours[i].Before(r.hours[j]) })
	r.mu.Lock()
	r.reset()
	r.mu.Unlock()
	return r
}

// reset preloads the days before StartDay and rebuilds the alert state (mu held).
func (r *Runner) reset() {
	r.mgr = NewManager(r.opts.ConfirmAfter)
	r.window = nil
	r.next, r.first = 0, 0
	r.running = false
	if len(r.hours) == 0 {
		return
	}
	cut := r.hours[0].Truncate(24 * time.Hour).Add(time.Duration(r.opts.StartDay-1) * 24 * time.Hour)
	for r.next < len(r.hours) && r.hours[r.next].Before(cut) {
		r.window = append(r.window, r.byHour[r.hours[r.next]]...)
		r.next++
	}
	r.first = r.next
	if r.next > 0 {
		res := r.analyze(r.hours[r.next-1])
		r.mgr.Update(r.hours[r.next-1], res.Findings)
	}
}

// analyze runs the batch engine over the window with the events already known at `now`.
func (r *Runner) analyze(now time.Time) analysis.Result {
	var known []domain.Event
	for _, e := range r.events {
		if !e.Timestamp.After(now) {
			known = append(known, e)
		}
	}
	return r.engine.Analyze(analysis.Input{Meters: r.meters, Readings: r.window, Events: known}, nil)
}

// Step streams the next simulated hour. It returns false when the replay has ended.
func (r *Runner) Step() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.step()
}

func (r *Runner) step() bool {
	if r.next >= len(r.hours) {
		return false
	}
	start := time.Now()
	h := r.hours[r.next]
	batch := r.byHour[h]
	r.window = append(r.window, batch...)
	r.next++
	res := r.analyze(h)
	changes := r.mgr.Update(h, res.Findings)
	r.lastTick = time.Since(start)
	if r.next >= len(r.hours) {
		r.running = false
	}

	pts := make([]Point, 0, len(batch))
	for _, x := range batch {
		pts = append(pts, point(x))
	}
	r.hub.Publish("tick", Tick{State: r.state(), Readings: pts, Status: r.statuses()})
	for _, c := range changes {
		r.hub.Publish("alert", c)
	}
	if r.next >= len(r.hours) {
		r.hub.Publish("control", r.state())
	}
	return true
}

func point(x domain.Reading) Point {
	return Point{MeterID: x.MeterID, Timestamp: x.Timestamp, KWh: x.ConsumptionKWh, VoltageV: x.VoltageV, CurrentA: x.CurrentA, PowerFactor: x.PowerFactor}
}

// statuses mirrors the batch rule: a confirmed real HIGH anomaly is critical, any other
// open alert is an alert, and a meter without an open alert is OK.
func (r *Runner) statuses() map[string]domain.MeterStatus {
	out := make(map[string]domain.MeterStatus, len(r.meters))
	for _, m := range r.meters {
		a := r.mgr.Active(m.ID)
		switch {
		case a == nil:
			out[m.ID] = domain.StatusOK
		case a.State == AlertConfirmed && a.Type == domain.RealAnomaly && a.Severity == domain.SeverityHigh:
			out[m.ID] = domain.StatusCritical
		default:
			out[m.ID] = domain.StatusAlert
		}
	}
	return out
}

func (r *Runner) state() State {
	s := State{Running: r.running, Speed: r.speed, StepMs: int64(float64(r.opts.Step.Milliseconds()) / r.speed), LastTickMs: float64(r.lastTick.Microseconds()) / 1000}
	if len(r.hours) == 0 {
		return s
	}
	s.Start = r.hours[r.first%len(r.hours)]
	s.End = r.hours[len(r.hours)-1]
	s.TotalHours = len(r.hours) - r.first
	s.Hour = r.next - r.first
	s.Finished = r.next >= len(r.hours)
	if r.next > 0 {
		s.Clock = r.hours[r.next-1]
	}
	return s
}

// State returns the clock and controls.
func (r *Runner) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state()
}

// Alerts returns the current alert list.
func (r *Runner) Alerts() []Alert {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mgr.Alerts()
}

func (r *Runner) snapshot() Snapshot {
	recent := map[string][]Point{}
	for i := len(r.window) - 1; i >= 0; i-- {
		x := r.window[i]
		if len(recent[x.MeterID]) < r.opts.Recent {
			recent[x.MeterID] = append(recent[x.MeterID], point(x))
		}
	}
	st := r.statuses()
	ms := make([]MeterLive, 0, len(r.meters))
	for _, m := range r.meters {
		pts := recent[m.ID]
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
		ms = append(ms, MeterLive{ID: m.ID, Name: m.Name, Status: st[m.ID], Recent: pts})
	}
	return Snapshot{State: r.state(), Meters: ms, Alerts: r.mgr.Alerts()}
}

// Subscribe registers a client. Holding the runner lock guarantees that no tick is
// published between the snapshot and the subscription: the client sees every change.
// When resumed is true, `missed` replaces the snapshot.
func (r *Runner) Subscribe(lastID uint64) (snap *Snapshot, snapID uint64, missed []Message, ch chan Message, cancel func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, missed, resumed, cancel := r.hub.Subscribe(lastID)
	if !resumed {
		s := r.snapshot()
		snap, missed = &s, nil
		snapID = r.hub.LastID()
	}
	return snap, snapID, missed, ch, cancel
}

// Control actions.
const (
	ActionStart = "start"
	ActionPause = "pause"
	ActionReset = "reset"
	ActionSpeed = "speed"
)

// Control applies start | pause | reset | speed and broadcasts the new state.
// A reset also broadcasts a fresh snapshot so every client starts over.
func (r *Runner) Control(action string, speed float64) (State, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch action {
	case ActionStart:
		if r.next >= len(r.hours) {
			r.reset()
		}
		r.running = true
	case ActionPause:
		r.running = false
	case ActionReset:
		r.reset()
		r.hub.Publish("snapshot", r.snapshot())
	case ActionSpeed:
		if speed < 0.25 || speed > 32 {
			return r.state(), false
		}
		r.speed = speed
	default:
		return r.state(), false
	}
	st := r.state()
	r.hub.Publish("control", st)
	select {
	case r.wake <- struct{}{}:
	default:
	}
	return st, true
}

// Close ends the open streams (used on server shutdown).
func (r *Runner) Close() { r.hub.Close() }

// Loop advances the clock in real time while running, until ctx ends. The wait
// counts from the start of the previous tick, so the work of a tick does not
// slow the replay down (8× means 8×).
func (r *Runner) Loop(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	last := time.Now()
	for {
		r.mu.Lock()
		running := r.running
		wait := time.Duration(float64(r.opts.Step)/r.speed) - time.Since(last)
		r.mu.Unlock()
		if wait < 0 {
			wait = 0
		}
		if !running {
			wait = time.Hour
		}
		timer.Reset(wait)
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			last = time.Now()
			r.mu.Lock()
			if r.running {
				r.step()
			}
			r.mu.Unlock()
		}
	}
}
