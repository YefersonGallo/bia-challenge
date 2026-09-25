// Package analysis is the anomaly engine. It is pure: it receives readings
// and events and returns statistics and classified findings, with no I/O.
//
// Pipeline (same order the UI shows):
//
//	readings → baseline → detection → correlation → events → (explanation) → recommendation
//
// The engine decides type, severity, confidence and priority deterministically.
// The LLM only writes the explanation from the evidence the engine produced.
package analysis

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Config holds the thresholds of the engine.
type Config struct {
	BaselineDays     int           // days used as reference window
	ShiftThreshold   float64       // hourly deviation (fraction) that counts as out of band
	MinEpisodeHours  int           // out-of-band hours needed to call it a sustained change
	EpisodeGapHours  int           // in-band hours tolerated inside an episode
	CriticalPct      float64       // variation that marks a meter as CRITICAL
	HighPct          float64       // shift above which a real anomaly is HIGH
	ZThreshold       float64       // robust z-score for spikes
	EventWindow      time.Duration // how close an event must be to the onset
	CoherenceTol     float64       // relative tolerance of kWh / (V·I·PF) against the meter's own ratio
	VoltageTol       float64       // voltage deviation from the meter's baseline considered implausible
	PFJump           float64       // power-factor jump against its neighbours considered implausible
	LocalWindow      int           // readings (centered) used as local reference for jumps
	MinInvalid       int           // suspicious readings that trigger DATA_QUALITY
	MinCoherence     float64       // physical coherence below which data is suspect
	PFDropThreshold  float64       // power-factor drop considered anomalous
	RelationShift    float64       // change of kWh / (V·I·PF) considered an anomalous electrical relation
	NightRatioSignal float64       // night consumption ratio worth reporting
	PersistenceHours float64       // hours after which persistence saturates in the priority score
}

// DefaultConfig returns the thresholds documented in the README.
func DefaultConfig() Config {
	return Config{
		BaselineDays:     7,
		ShiftThreshold:   0.25,
		MinEpisodeHours:  6,
		EpisodeGapHours:  2,
		CriticalPct:      100,
		HighPct:          50,
		ZThreshold:       3.5,
		EventWindow:      24 * time.Hour,
		CoherenceTol:     0.25,
		VoltageTol:       0.06,
		PFJump:           0.15,
		LocalWindow:      7,
		MinInvalid:       5,
		MinCoherence:     0.90,
		PFDropThreshold:  0.05,
		RelationShift:    0.15,
		NightRatioSignal: 1.5,
		PersistenceHours: 48,
	}
}

// Input is everything the engine needs.
type Input struct {
	Meters   []domain.Meter
	Readings []domain.Reading
	Events   []domain.Event
}

// DayPoint is one day of a meter compared with its expected consumption.
type DayPoint struct {
	Day          int     `json:"day"`
	Date         string  `json:"date"`
	KWh          float64 `json:"kwh"`
	ExpectedKWh  float64 `json:"expected_kwh"`
	DeviationPct float64 `json:"deviation_pct"`
	Invalid      int     `json:"invalid"`
	VoltageV     float64 `json:"voltage_v"`
	CurrentA     float64 `json:"current_a"`
	PowerFactor  float64 `json:"power_factor"`
}

// Episode is a sustained deviation from the hourly baseline.
type Episode struct {
	OnsetDay   int       `json:"onset_day"`
	EndDay     int       `json:"end_day"`
	Onset      time.Time `json:"onset"`
	End        time.Time `json:"end"`
	Direction  int       `json:"direction"` // +1 increase, -1 decrease
	Hours      int       `json:"hours"`
	Recovered  bool      `json:"recovered"`
	MeanDevPct float64   `json:"mean_deviation_pct"`
}

// MeterStats is the rule layer: statistics available without any AI.
//
// BaselineKWh is the expected consumption of a day (sum of the hour-of-day
// medians of the baseline window) and CurrentKWh the consumption of the last
// 24 hours, so VariationPct describes the meter's present state.
type MeterStats struct {
	MeterID          string             `json:"meter_id"`
	Readings         int                `json:"readings"`
	PeriodKWh        float64            `json:"period_kwh"`
	BaselineKWh      float64            `json:"baseline_kwh"`
	CurrentKWh       float64            `json:"current_kwh"`
	VariationPct     float64            `json:"variation_pct"`
	Status           domain.MeterStatus `json:"status"`
	StatusReason     string             `json:"status_reason"`
	InvalidReadings  int                `json:"invalid_readings"` // suspicious readings of any kind
	PFOutOfRange     int                `json:"pf_out_of_range"`
	ZeroVoltage      int                `json:"zero_voltage"`
	VoltageAnomalies int                `json:"voltage_anomalies"`
	PFJumps          int                `json:"pf_jumps"`
	Incoherent       int                `json:"incoherent_readings"`
	Coherence        float64            `json:"physical_coherence"`
	IssueOnset       *time.Time         `json:"issue_onset,omitempty"`
	IssueHours       int                `json:"issue_hours"`
	VoltageRange     [2]float64         `json:"voltage_range"` // min/max of suspicious voltages
	PFRange          [2]float64         `json:"pf_range"`      // min/max of suspicious power factors
	Days             []DayPoint         `json:"days"`
	HourlyBaseline   [24]float64        `json:"hourly_baseline"`
	HourlyCurrent    [24]float64        `json:"hourly_current"`
	NightRatio       float64            `json:"night_ratio"`
	Spikes           int                `json:"spikes"`
	Episode          *Episode           `json:"episode,omitempty"`
	RelationShiftPct float64            `json:"relation_shift_pct"`
	Base             Electrical         `json:"base_electrical"`
	Cur              Electrical         `json:"current_electrical"`
}

// Electrical summarizes mean electrical values of a window.
type Electrical struct {
	KWhPerDay   float64 `json:"kwh_per_day"`
	VoltageV    float64 `json:"voltage_v"`
	CurrentA    float64 `json:"current_a"`
	PowerFactor float64 `json:"power_factor"`
}

// QualityIssue reports whether the meter's readings cannot be trusted.
func (s MeterStats) QualityIssue(c Config) bool {
	return s.InvalidReadings >= c.MinInvalid || s.Coherence < c.MinCoherence
}

// Finding is a classified anomaly before explanation.
type Finding struct {
	Anomaly domain.Anomaly
	Stats   MeterStats
}

// Result of a run of the engine.
type Result struct {
	Stats    map[string]MeterStats
	Findings []Finding // sorted by priority, rank set
	Readings int
	Invalid  int
}

// Observer receives progress for each pipeline step.
type Observer func(stepKey, result string)

// Engine runs the analysis with a configuration.
type Engine struct{ cfg Config }

// New returns an engine.
func New(cfg Config) *Engine { return &Engine{cfg: cfg} }

// Config exposes the thresholds (read-only copy).
func (e *Engine) Config() Config { return e.cfg }

// Analyze runs the deterministic part of the pipeline.
func (e *Engine) Analyze(in Input, obs Observer) Result {
	if obs == nil {
		obs = func(string, string) {}
	}
	byMeter := groupReadings(in.Readings)

	// 1. readings + quality
	res := Result{Stats: map[string]MeterStats{}, Readings: len(in.Readings)}
	for _, m := range in.Meters {
		s := e.ComputeStats(byMeter[m.ID])
		s.MeterID = m.ID
		res.Stats[m.ID] = s
		res.Invalid += s.InvalidReadings
	}
	obs("readings", fmt.Sprintf("%s lecturas · %d inconsistentes", fmtInt(len(in.Readings)), res.Invalid))

	// 2. baseline (computed inside ComputeStats)
	obs("baseline", fmt.Sprintf("%d perfiles horarios · días 1–%d", len(in.Meters), e.cfg.BaselineDays))

	// 3. detection
	shifts := 0
	for _, s := range res.Stats {
		if s.Episode != nil && !s.QualityIssue(e.cfg) {
			shifts++
		}
	}
	obs("detection", fmt.Sprintf("%d desviaciones sostenidas", shifts))

	// 4. correlation
	anomalousElec := 0
	for _, s := range res.Stats {
		if (s.Episode != nil && e.electricalAnomaly(s)) || s.QualityIssue(e.cfg) {
			anomalousElec++
		}
	}
	obs("correlation", fmt.Sprintf("%d relaciones eléctricas anómalas", anomalousElec))

	// 5. events + classification
	coherentEvents := 0
	for _, m := range in.Meters {
		f := e.Classify(m, res.Stats[m.ID], in.Events)
		if f == nil {
			continue
		}
		if f.Anomaly.Evidence.EventExplainsShift {
			coherentEvents++
		}
		res.Findings = append(res.Findings, *f)
	}
	obs("events", fmt.Sprintf("%d eventos coherentes", coherentEvents))

	Prioritize(res.Findings)
	return res
}

func groupReadings(rs []domain.Reading) map[string][]domain.Reading {
	out := map[string][]domain.Reading{}
	for _, r := range rs {
		out[r.MeterID] = append(out[r.MeterID], r)
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool { return out[k][i].Timestamp.Before(out[k][j].Timestamp) })
	}
	return out
}

func (e *Engine) invalid(r domain.Reading) (pfBad, zeroV bool) {
	pfBad = r.PowerFactor > 1 || r.PowerFactor < 0
	zeroV = r.VoltageV <= 0 && r.ConsumptionKWh > 0
	return
}

// localMedian returns, for each index, the median of the valid values in a
// centered window (excluding NaN).
func localMedian(xs []float64, window int) []float64 {
	half := window / 2
	out := make([]float64, len(xs))
	buf := make([]float64, 0, window)
	for i := range xs {
		buf = buf[:0]
		for j := max(0, i-half); j <= min(len(xs)-1, i+half); j++ {
			if !math.IsNaN(xs[j]) {
				buf = append(buf, xs[j])
			}
		}
		if len(buf) == 0 {
			out[i] = math.NaN()
		} else {
			out[i] = median(buf)
		}
	}
	return out
}

// ComputeStats builds the rule-layer statistics of one meter (readings sorted).
func (e *Engine) ComputeStats(rs []domain.Reading) MeterStats {
	s := MeterStats{Status: domain.StatusOK, StatusReason: "dentro del rango esperado", Coherence: 1}
	n := len(rs)
	if n == 0 {
		return s
	}
	c := e.cfg
	s.Readings = n
	start := rs[0].Timestamp.Truncate(24 * time.Hour)
	dayOf := func(t time.Time) int { return int(t.Sub(start).Hours())/24 + 1 }
	isBase := func(i int) bool { return dayOf(rs[i].Timestamp) <= c.BaselineDays }

	// --- 1. plausibility of every reading ---------------------------------------
	hard := make([]bool, n)
	ratio := make([]float64, n) // kWh / (V·I·PF/1000): the meter's own physical relation
	pf := make([]float64, n)
	for i, r := range rs {
		pfBad, zeroV := e.invalid(r)
		hard[i] = pfBad || zeroV
		if pfBad {
			s.PFOutOfRange++
		}
		if zeroV {
			s.ZeroVoltage++
		}
		ratio[i], pf[i] = math.NaN(), math.NaN()
		if !hard[i] {
			pf[i] = r.PowerFactor
			if p := r.VoltageV * r.CurrentA * r.PowerFactor / 1000; p > 0 {
				ratio[i] = r.ConsumptionKWh / p
			}
		}
	}
	var baseRatios, baseVolts []float64
	for i, r := range rs {
		if isBase(i) && !hard[i] {
			if !math.IsNaN(ratio[i]) {
				baseRatios = append(baseRatios, ratio[i])
			}
			baseVolts = append(baseVolts, r.VoltageV)
		}
	}
	baseRatio, baseVolt := median(baseRatios), median(baseVolts)
	localRatio, localPF := localMedian(ratio, c.LocalWindow), localMedian(pf, c.LocalWindow)

	sus := make([]bool, n)
	checked, coherent := 0, 0
	first, last := -1, -1
	vr, pr := [2]float64{math.Inf(1), math.Inf(-1)}, [2]float64{math.Inf(1), math.Inf(-1)}
	for i, r := range rs {
		vAnom := !hard[i] && baseVolt > 0 && math.Abs(r.VoltageV/baseVolt-1) > c.VoltageTol
		pfJ := !hard[i] && !math.IsNaN(localPF[i]) && math.Abs(r.PowerFactor-localPF[i]) > c.PFJump
		incoh := false
		if !math.IsNaN(ratio[i]) && baseRatio > 0 {
			checked++
			// Incoherent only if it breaks both the meter's usual relation and its
			// neighbours: a persistent regime change (a real fault) is not a data error.
			incoh = math.Abs(ratio[i]/baseRatio-1) > c.CoherenceTol && math.Abs(ratio[i]/localRatio[i]-1) > c.CoherenceTol
			if !incoh {
				coherent++
			}
		}
		if vAnom {
			s.VoltageAnomalies++
		}
		if pfJ {
			s.PFJumps++
		}
		if incoh {
			s.Incoherent++
		}
		sus[i] = hard[i] || vAnom || pfJ || incoh
		if sus[i] {
			s.InvalidReadings++
			if first < 0 {
				first = i
			}
			last = i
			vr[0], vr[1] = math.Min(vr[0], r.VoltageV), math.Max(vr[1], r.VoltageV)
			pr[0], pr[1] = math.Min(pr[0], r.PowerFactor), math.Max(pr[1], r.PowerFactor)
		}
	}
	if checked > 0 {
		s.Coherence = round(float64(coherent)/float64(checked), 3)
	}
	if first >= 0 {
		// The issue starts where suspicious readings cluster (≥ 3 in 24 h), so an
		// isolated noisy reading does not move the onset.
		for i := first; i <= last; i++ {
			if !sus[i] {
				continue
			}
			k := 0
			for j := i; j < n && rs[j].Timestamp.Sub(rs[i].Timestamp) < 24*time.Hour; j++ {
				if sus[j] {
					k++
				}
			}
			if k >= 3 {
				first = i
				break
			}
		}
		t := rs[first].Timestamp
		s.IssueOnset = &t
		s.IssueHours = int(rs[last].Timestamp.Sub(t).Hours()) + 1
		s.VoltageRange = [2]float64{round(vr[0], 1), round(vr[1], 1)}
		s.PFRange = [2]float64{round(pr[0], 2), round(pr[1], 2)}
	}

	// --- 2. hour-of-day baseline (median + MAD over plausible readings) ---------
	perHourBase := make([][]float64, 24)
	for i, r := range rs {
		s.PeriodKWh += r.ConsumptionKWh
		if isBase(i) && !sus[i] {
			perHourBase[r.Timestamp.Hour()] = append(perHourBase[r.Timestamp.Hour()], r.ConsumptionKWh)
		}
	}
	medians, mads := [24]float64{}, [24]float64{}
	expectedDay := 0.0
	for h := 0; h < 24; h++ {
		medians[h] = median(perHourBase[h])
		mads[h] = mad(perHourBase[h], medians[h])
		s.HourlyBaseline[h] = round(medians[h], 3)
		expectedDay += medians[h]
	}
	floor := 0.1 * expectedDay / 24
	expectedAt := func(h int) float64 { return math.Max(medians[h], floor) }

	// --- 3. current state: last 24 hours ----------------------------------------
	lastTS := rs[n-1].Timestamp
	cur24 := func(i int) bool { return lastTS.Sub(rs[i].Timestamp) < 24*time.Hour }
	var curCount [24]int
	for i, r := range rs {
		if cur24(i) {
			s.CurrentKWh += r.ConsumptionKWh
			h := r.Timestamp.Hour()
			s.HourlyCurrent[h] += r.ConsumptionKWh
			curCount[h]++
		}
	}
	for h := range s.HourlyCurrent {
		if curCount[h] > 0 {
			s.HourlyCurrent[h] = round(s.HourlyCurrent[h]/float64(curCount[h]), 3)
		}
	}
	s.BaselineKWh = round(expectedDay, 1)
	s.VariationPct = round(pctChange(expectedDay, s.CurrentKWh), 1)
	s.CurrentKWh = round(s.CurrentKWh, 1)
	s.PeriodKWh = round(s.PeriodKWh, 1)

	// night pattern (22:00–05:59) of the last 24 h against the baseline
	var nb, nc []float64
	for _, h := range []int{22, 23, 0, 1, 2, 3, 4, 5} {
		nb = append(nb, medians[h])
		if curCount[h] > 0 {
			nc = append(nc, s.HourlyCurrent[h])
		}
	}
	if mean(nb) > 0 && len(nc) > 0 {
		s.NightRatio = round(mean(nc)/mean(nb), 2)
	}

	// --- 4. daily points ----------------------------------------------------------
	type acc struct{ v, i, pf []float64 }
	days := map[int]*DayPoint{}
	elec := map[int]*acc{}
	for i, r := range rs {
		d := dayOf(r.Timestamp)
		dp := days[d]
		if dp == nil {
			dp = &DayPoint{Day: d, Date: start.AddDate(0, 0, d-1).Format("2006-01-02")}
			days[d], elec[d] = dp, &acc{}
		}
		dp.KWh += r.ConsumptionKWh
		if sus[i] {
			dp.Invalid++
			continue
		}
		elec[d].v = append(elec[d].v, r.VoltageV)
		elec[d].i = append(elec[d].i, r.CurrentA)
		elec[d].pf = append(elec[d].pf, r.PowerFactor)
	}
	keys := make([]int, 0, len(days))
	for d := range days {
		keys = append(keys, d)
	}
	sort.Ints(keys)
	for _, d := range keys {
		dp := days[d]
		dp.ExpectedKWh = round(expectedDay, 2)
		dp.DeviationPct = round(pctChange(expectedDay, dp.KWh), 1)
		dp.KWh = round(dp.KWh, 2)
		dp.VoltageV, dp.CurrentA, dp.PowerFactor = round(mean(elec[d].v), 1), round(mean(elec[d].i), 2), round(mean(elec[d].pf), 3)
		s.Days = append(s.Days, *dp)
	}

	// --- 5. sustained change: hourly deviations after the baseline window -------
	dev := make([]float64, n)
	for i, r := range rs {
		dev[i] = math.NaN()
		if !isBase(i) && !sus[i] && expectedDay > 0 {
			dev[i] = r.ConsumptionKWh/expectedAt(r.Timestamp.Hour()) - 1
		}
	}
	s.Episode = e.findEpisode(rs, dev, dayOf)

	// --- 6. electrical relation: baseline window vs change window ---------------
	inWindow := cur24
	if ep := s.Episode; ep != nil {
		inWindow = func(i int) bool { t := rs[i].Timestamp; return !t.Before(ep.Onset) && !t.After(ep.End) }
	}
	var bv, bi, bpf, cv, ci, cpf, ck, cr []float64
	for i, r := range rs {
		if sus[i] {
			continue
		}
		if isBase(i) {
			bv, bi, bpf = append(bv, r.VoltageV), append(bi, r.CurrentA), append(bpf, r.PowerFactor)
		} else if inWindow(i) {
			cv, ci, cpf, ck = append(cv, r.VoltageV), append(ci, r.CurrentA), append(cpf, r.PowerFactor), append(ck, r.ConsumptionKWh)
			if !math.IsNaN(ratio[i]) {
				cr = append(cr, ratio[i])
			}
		}
	}
	s.Base = Electrical{KWhPerDay: round(expectedDay, 2), VoltageV: round(mean(bv), 1), CurrentA: round(mean(bi), 2), PowerFactor: round(mean(bpf), 3)}
	s.Cur = Electrical{KWhPerDay: round(mean(ck)*24, 2), VoltageV: round(mean(cv), 1), CurrentA: round(mean(ci), 2), PowerFactor: round(mean(cpf), 3)}
	if baseRatio > 0 && len(cr) > 0 {
		s.RelationShiftPct = round(pctChange(baseRatio, median(cr)), 1)
	}

	// --- 7. spikes: isolated hours far from their hour-of-day baseline ----------
	for i := range rs {
		if isBase(i) || sus[i] || (s.Episode != nil && inWindow(i)) {
			continue
		}
		out := func(j int) bool {
			if j < 0 || j >= n {
				return false
			}
			h := rs[j].Timestamp.Hour()
			return math.Abs(robustZ(rs[j].ConsumptionKWh, medians[h], mads[h])) >= c.ZThreshold
		}
		if out(i) && !out(i-1) && !out(i+1) {
			s.Spikes++
		}
	}

	s.Status, s.StatusReason = e.status(s)
	return s
}

// findEpisode looks for the most significant run of out-of-band hours after
// the baseline window, tolerating short in-band gaps.
func (e *Engine) findEpisode(rs []domain.Reading, dev []float64, dayOf func(time.Time) int) *Episode {
	c := e.cfg
	type run struct {
		dir, start, end, n int
		sum                float64
	}
	var runs []run
	var cur *run
	gap := 0
	flush := func() {
		if cur != nil && cur.n >= c.MinEpisodeHours {
			runs = append(runs, *cur)
		}
		cur, gap = nil, 0
	}
	for i := range rs {
		if math.IsNaN(dev[i]) {
			if cur != nil && gap < c.EpisodeGapHours {
				gap++
				continue
			}
			flush()
			continue
		}
		sgn := 0
		if dev[i] >= c.ShiftThreshold {
			sgn = 1
		} else if dev[i] <= -c.ShiftThreshold {
			sgn = -1
		}
		if cur != nil && sgn == cur.dir {
			cur.end, cur.n, cur.sum, gap = i, cur.n+1, cur.sum+dev[i], 0
			continue
		}
		if cur != nil && gap < c.EpisodeGapHours {
			gap++
			continue
		}
		flush()
		if sgn != 0 {
			cur = &run{dir: sgn, start: i, end: i, n: 1, sum: dev[i]}
		}
	}
	flush()
	if len(runs) == 0 {
		return nil
	}
	best := runs[0]
	score := func(r run) float64 { return float64(r.n) * math.Abs(r.sum/float64(r.n)) }
	for _, r := range runs[1:] {
		if score(r) > score(best) {
			best = r
		}
	}
	onset, end := rs[best.start].Timestamp, rs[best.end].Timestamp
	return &Episode{
		OnsetDay: dayOf(onset), EndDay: dayOf(end), Onset: onset, End: end, Direction: best.dir,
		Hours:      int(end.Sub(onset).Hours()) + 1,
		Recovered:  best.end < len(rs)-1-c.EpisodeGapHours,
		MeanDevPct: round(best.sum/float64(best.n)*100, 1),
	}
}

func (e *Engine) status(s MeterStats) (domain.MeterStatus, string) {
	ep := s.Episode
	switch {
	case s.VariationPct >= e.cfg.CriticalPct:
		return domain.StatusCritical, fmt.Sprintf("variación %s sobre el baseline", fmtPct(s.VariationPct))
	case s.QualityIssue(e.cfg):
		return domain.StatusAlert, fmt.Sprintf("%d lecturas físicamente inconsistentes", s.InvalidReadings)
	case math.Abs(s.VariationPct) >= e.cfg.ShiftThreshold*100:
		return domain.StatusAlert, fmt.Sprintf("variación %s frente al baseline", fmtPct(s.VariationPct))
	case ep != nil:
		return domain.StatusAlert, fmt.Sprintf("%s de %s durante %d h (día %d)", map[int]string{1: "aumento", -1: "caída"}[ep.Direction], fmtPct(ep.MeanDevPct), ep.Hours, ep.OnsetDay)
	default:
		return domain.StatusOK, "dentro del rango esperado"
	}
}

func (e *Engine) electricalAnomaly(s MeterStats) bool {
	pfDrop := s.Base.PowerFactor - s.Cur.PowerFactor
	iChg := pctChange(s.Base.CurrentA, s.Cur.CurrentA)
	kChg := pctChange(s.Base.KWhPerDay, s.Cur.KWhPerDay)
	return pfDrop >= e.cfg.PFDropThreshold || math.Abs(iChg-kChg) > 20 || math.Abs(s.RelationShiftPct) >= e.cfg.RelationShift*100
}

// Classify applies the decision tree: data quality → events → magnitude.
func (e *Engine) Classify(m domain.Meter, s MeterStats, events []domain.Event) *Finding {
	ev := domain.Evidence{
		MeterID: m.ID, MeterName: m.Name, BaselineKWh: s.BaselineKWh, CurrentKWh: s.CurrentKWh, VariationPct: s.VariationPct,
		NightRatio: s.NightRatio, InvalidReadings: s.InvalidReadings, PhysicalCoherence: s.Coherence,
		Variables: e.variables(s),
	}
	a := domain.Anomaly{MeterID: m.ID, Status: domain.AnomalyOpen}

	switch {
	case s.QualityIssue(e.cfg):
		e.classifyDataQuality(&a, &ev, s, meterEvents(m.ID, events))
	case s.Episode != nil:
		e.classifyEpisode(&a, &ev, s, events)
	default:
		return nil
	}

	a.Evidence = ev
	a.Confidence = e.confidence(a, s)
	return &Finding{Anomaly: a, Stats: s}
}

func meterEvents(id string, events []domain.Event) []domain.Event {
	var out []domain.Event
	for _, ev := range events {
		if ev.MeterID == id {
			out = append(out, ev)
		}
	}
	return out
}

func (e *Engine) classifyDataQuality(a *domain.Anomaly, ev *domain.Evidence, s MeterStats, events []domain.Event) {
	a.Type, a.Severity = domain.DataQuality, domain.SeverityMedium
	if s.InvalidReadings >= 10 || s.VoltageAnomalies >= 3 || s.Coherence < e.cfg.MinCoherence {
		a.Severity = domain.SeverityHigh
	}
	if s.IssueOnset != nil {
		t := *s.IssueOnset
		ev.Onset = &t
		ev.OnsetDay = dayIndex(s, t)
	}
	ev.PersistentHours = s.IssueHours
	if s.VoltageAnomalies > 0 {
		ev.Signals = append(ev.Signals, sig("VOLTAGE_JUMPS", fmt.Sprintf("%d lecturas con voltaje fuera de ±%s del habitual (%s–%s V)", s.VoltageAnomalies, fmtPctPlain(e.cfg.VoltageTol*100), fmtNum(s.VoltageRange[0], 0), fmtNum(s.VoltageRange[1], 0)), float64(s.VoltageAnomalies)))
	}
	if s.PFJumps > 0 {
		ev.Signals = append(ev.Signals, sig("PF_JUMPS", fmt.Sprintf("%d saltos bruscos del factor de potencia (%s–%s)", s.PFJumps, fmtNum(s.PFRange[0], 2), fmtNum(s.PFRange[1], 2)), float64(s.PFJumps)))
	}
	if s.Incoherent > 0 {
		ev.Signals = append(ev.Signals, sig("INCOHERENT_READINGS", fmt.Sprintf("%d lecturas donde el consumo no cuadra con V·I·PF", s.Incoherent), float64(s.Incoherent)))
	}
	if s.PFOutOfRange > 0 {
		ev.Signals = append(ev.Signals, sig("PF_OUT_OF_RANGE", fmt.Sprintf("%d lecturas con factor de potencia fuera de [0, 1]", s.PFOutOfRange), float64(s.PFOutOfRange)))
	}
	if s.ZeroVoltage > 0 {
		ev.Signals = append(ev.Signals, sig("ZERO_VOLTAGE", fmt.Sprintf("%d horas con 0 V y consumo positivo", s.ZeroVoltage), float64(s.ZeroVoltage)))
	}
	if math.Abs(s.VariationPct) < e.cfg.ShiftThreshold*100 && s.Episode == nil {
		ev.Signals = append(ev.Signals, sig("STABLE_CONSUMPTION", fmt.Sprintf("Consumo estable (%s): no es un cambio de carga", fmtPct(s.VariationPct)), s.VariationPct))
	}
	for _, x := range events {
		if x.Kind() == domain.EventDataQuality {
			ev.RelatedEvents = append(ev.RelatedEvents, x)
			ev.Signals = append(ev.Signals, sig("DQ_EVENT", fmt.Sprintf("Registro operativo: «%s» (%s)", x.Description, x.Timestamp.Format("02/01 15:04")), 1))
		}
	}
}

// dayIndex returns the 1-based day of t in the meter's daily series.
func dayIndex(s MeterStats, t time.Time) int {
	d := t.Format("2006-01-02")
	for _, p := range s.Days {
		if p.Date == d {
			return p.Day
		}
	}
	return 0
}

func (e *Engine) classifyEpisode(a *domain.Anomaly, ev *domain.Evidence, s MeterStats, events []domain.Event) {
	ep := s.Episode
	onset := ep.Onset
	ev.Onset = &onset
	ev.OnsetDay, ev.EndDay, ev.PersistentHours, ev.ShiftPct = ep.OnsetDay, ep.EndDay, ep.Hours, ep.MeanDevPct
	related, coherent := e.matchEvents(a.MeterID, ep, events)
	ev.RelatedEvents = related
	verb := "por encima"
	if ep.Direction < 0 {
		verb = "por debajo"
	}
	ev.Signals = append(ev.Signals, sig("PERSISTENT_SHIFT", fmt.Sprintf("%d h %s de la banda (±%s) desde el día %d a las %s, con desvío medio de %s", ep.Hours, verb, fmtPctPlain(e.cfg.ShiftThreshold*100), ep.OnsetDay, ep.Onset.Format("15:04"), fmtPct(ep.MeanDevPct)), float64(ep.Hours)))
	elec := e.electricalAnomaly(s)

	if coherent != nil {
		ev.EventExplainsShift = true
		ev.Signals = append(ev.Signals, sig("EVENT_COHERENT", fmt.Sprintf("Evento coherente con el cambio: «%s» (%s)", coherent.Description, coherent.Timestamp.Format("02/01 15:04")), 1))
		if coherent.Kind() == domain.EventShutdown && ep.Recovered {
			a.Type, a.Severity = domain.FalsePositive, domain.SeverityLow
			ev.Signals = append(ev.Signals, sig("RECOVERED", fmt.Sprintf("El consumo volvió a su nivel al terminar el evento (%s)", ep.End.Add(time.Hour).Format("02/01 15:04")), float64(ep.Hours)))
		} else {
			a.Type, a.Severity = domain.ExplainableAnomaly, domain.SeverityMedium
		}
		if !elec {
			ev.Signals = append(ev.Signals, sig("HEALTHY_ELECTRICAL", fmt.Sprintf("Relación eléctrica sana: la corriente acompaña al consumo y el factor de potencia se mantiene en %s", fmtNum(s.Cur.PowerFactor, 2)), s.Cur.PowerFactor))
		}
		return
	}

	a.Type, a.Severity = domain.RealAnomaly, domain.SeverityMedium
	if math.Abs(ep.MeanDevPct) >= e.cfg.HighPct || elec {
		a.Severity = domain.SeverityHigh
	}
	if len(related) > 0 {
		x := related[0]
		ev.Signals = append(ev.Signals, sig("NO_EVENT", fmt.Sprintf("El registro de ±%d h no explica el cambio: «%s» (%s)", int(e.cfg.EventWindow.Hours()), x.Description, x.Type), 0))
	} else {
		ev.Signals = append(ev.Signals, sig("NO_EVENT", fmt.Sprintf("Ningún evento operativo en ±%d h del inicio del cambio", int(e.cfg.EventWindow.Hours())), 0))
	}
	if iChg := pctChange(s.Base.CurrentA, s.Cur.CurrentA); math.Abs(iChg) > 25 {
		ev.Signals = append(ev.Signals, sig("CURRENT_RISE", fmt.Sprintf("Corriente media de %s a %s A (%s)", fmtNum(s.Base.CurrentA, 0), fmtNum(s.Cur.CurrentA, 0), fmtPct(iChg)), round(iChg, 1)))
	}
	if pfDrop := s.Base.PowerFactor - s.Cur.PowerFactor; pfDrop >= e.cfg.PFDropThreshold {
		ev.Signals = append(ev.Signals, sig("PF_DROP", fmt.Sprintf("Factor de potencia de %s a %s", fmtNum(s.Base.PowerFactor, 2), fmtNum(s.Cur.PowerFactor, 2)), round(pfDrop, 2)))
	}
	if math.Abs(s.RelationShiftPct) >= e.cfg.RelationShift*100 {
		ev.Signals = append(ev.Signals, sig("RELATION_SHIFT", fmt.Sprintf("La relación entre consumo y V·I·PF cambió %s: el equipo no se comporta como antes", fmtPct(s.RelationShiftPct)), s.RelationShiftPct))
	}
	if s.NightRatio >= e.cfg.NightRatioSignal {
		ev.Signals = append(ev.Signals, sig("NIGHT_PATTERN", fmt.Sprintf("Consumo nocturno %s× el baseline", fmtNum(s.NightRatio, 1)), s.NightRatio))
	}
	if s.Spikes > 0 {
		ev.Signals = append(ev.Signals, sig("SPIKES", fmt.Sprintf("%d %s (|z| > %s)", s.Spikes, plural(s.Spikes, "pico aislado", "picos aislados"), fmtNum(e.cfg.ZThreshold, 1)), float64(s.Spikes)))
	}
}

// matchEvents returns events near the onset and the first one whose effect
// is coherent with the direction of the change.
func (e *Engine) matchEvents(meterID string, ep *Episode, events []domain.Event) ([]domain.Event, *domain.Event) {
	var related []domain.Event
	var coherent *domain.Event
	for _, ev := range events {
		if ev.MeterID != meterID {
			continue
		}
		diff := ev.Timestamp.Sub(ep.Onset)
		if diff < 0 {
			diff = -diff
		}
		if diff > e.cfg.EventWindow {
			continue
		}
		related = append(related, ev)
		k := ev.Kind()
		ok := (ep.Direction > 0 && k == domain.EventLoadIncrease) || (ep.Direction < 0 && (k == domain.EventShutdown || k == domain.EventLoadDecrease))
		if ok && coherent == nil {
			c := ev
			coherent = &c
		}
	}
	return related, coherent
}

func (e *Engine) variables(s MeterStats) []domain.VariableChange {
	v := func(name, unit string, b, c float64) domain.VariableChange {
		return domain.VariableChange{Name: name, Unit: unit, Baseline: b, Current: c, DeltaPct: round(pctChange(b, c), 1)}
	}
	return []domain.VariableChange{
		v("Consumo diario", "kWh", s.BaselineKWh, s.Cur.KWhPerDay),
		v("Corriente media", "A", s.Base.CurrentA, s.Cur.CurrentA),
		v("Factor de potencia", "", s.Base.PowerFactor, s.Cur.PowerFactor),
		v("Voltaje medio", "V", s.Base.VoltageV, s.Cur.VoltageV),
	}
}

// confidence combines the number of corroborating signals with the strength
// of the main one. It is bounded to [0.5, 0.97]: the engine never claims certainty.
func (e *Engine) confidence(a domain.Anomaly, s MeterStats) float64 {
	n := float64(len(a.Evidence.Signals))
	var strength float64
	switch a.Type {
	case domain.RealAnomaly:
		strength = clamp(math.Abs(a.Evidence.ShiftPct)/100, 0, 1)
	case domain.DataQuality:
		strength = clamp(float64(s.InvalidReadings)/20+(1-s.Coherence)*2, 0, 1)
	case domain.ExplainableAnomaly:
		strength = 0.8
	case domain.FalsePositive:
		strength = 0.3
	}
	return round(clamp(0.5+0.07*n+0.2*strength, 0.5, 0.97), 2)
}

// Prioritize computes the priority score and rank of each finding:
//
//	severity × magnitude × persistence × (1 − explained)
func Prioritize(fs []Finding) {
	cfg := DefaultConfig()
	for i := range fs {
		a := &fs[i].Anomaly
		s := fs[i].Stats
		mag := clamp(math.Abs(a.Evidence.ShiftPct)/100, 0, 1)
		pers := clamp(float64(a.Evidence.PersistentHours)/cfg.PersistenceHours, 0, 1)
		if a.Type == domain.DataQuality {
			mag = clamp(0.5+float64(s.InvalidReadings)/40+(1-s.Coherence), 0, 1)
		}
		explained := 0.0
		if a.Evidence.EventExplainsShift {
			explained = 0.8
		}
		a.PriorityScore = round(a.Severity.Weight()*mag*pers*(1-explained), 3)
	}
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Anomaly.PriorityScore > fs[j].Anomaly.PriorityScore })
	for i := range fs {
		fs[i].Anomaly.Rank = i + 1
	}
}

func sig(code, desc string, v float64) domain.Signal {
	return domain.Signal{Code: code, Description: desc, Value: v}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
