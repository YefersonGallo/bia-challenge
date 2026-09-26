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
	EventWindow      time.Duration // how close an event must be to the change point
	DurationTol      float64       // hours of tolerance between an outage's stated and observed duration
	CoherenceTol     float64       // relative tolerance of kWh / (V·I·PF) against the meter's own ratio
	VoltageMin       float64       // plausible voltage band (V)
	VoltageMax       float64
	VoltageJump      float64 // voltage step between consecutive hours considered implausible (V)
	PFJump           float64 // power-factor jump against its neighbours considered implausible
	LocalWindow      int     // readings (centered) used as local reference for jumps
	MinInvalid       int     // suspicious readings that trigger DATA_QUALITY
	MinCoherence     float64 // physical coherence below which data is suspect
	PFDropThreshold  float64 // power-factor drop considered anomalous
	RelationShift    float64 // change of kWh / (V·I·PF) considered an anomalous electrical relation
	NightRatioSignal float64 // night consumption ratio worth reporting
	PersistenceHours float64 // hours after which persistence saturates in the confidence
	DQHighShare      float64 // share of flagged readings in 24 h that makes data quality HIGH
	TariffCOPPerKWh  float64 // energy tariff used for the impact (COP per kWh)
	ReactiveLimit    float64 // kVArh/kWh above which reactive energy is billed (0.5)
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
		EventWindow:      3 * time.Hour,
		DurationTol:      2,
		CoherenceTol:     0.25,
		VoltageMin:       209,
		VoltageMax:       231,
		VoltageJump:      15,
		PFJump:           0.15,
		LocalWindow:      7,
		MinInvalid:       5,
		MinCoherence:     0.90,
		PFDropThreshold:  0.05,
		RelationShift:    0.15,
		NightRatioSignal: 1.5,
		PersistenceHours: 24,
		DQHighShare:      0.10,
		TariffCOPPerKWh:  850,
		ReactiveLimit:    0.5,
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
	VoltageRange     [2]float64         `json:"voltage_range"`   // min/max of suspicious voltages
	PFRange          [2]float64         `json:"pf_range"`        // min/max of the power factors that jumped
	MissingHours     int                `json:"missing_hours"`   // hours without a reading between the first and the last one
	DuplicateHours   int                `json:"duplicate_hours"` // repeated readings of the same hour (only the first one is used)
	Days             []DayPoint         `json:"days"`
	HourlyBaseline   [24]float64        `json:"hourly_baseline"`
	HourlyCurrent    [24]float64        `json:"hourly_current"`
	NightRatio       float64            `json:"night_ratio"`
	Spikes           int                `json:"spikes"`
	Episode          *Episode           `json:"episode,omitempty"`
	RelationShiftPct float64            `json:"relation_shift_pct"`
	VoltageJumps     int                `json:"voltage_jumps"`
	Flagged          []FlaggedReading   `json:"flagged_readings"`
	MaxFlagShare24h  float64            `json:"max_flag_share_24h"`
	KFactor          float64            `json:"k_factor"` // median kWh / (V·I·PF/1000) of the baseline window
	KMAD             float64            `json:"k_mad"`
	HourlyP10        [24]float64        `json:"hourly_p10"`
	HourlyP90        [24]float64        `json:"hourly_p90"`
	LevelFactor      float64            `json:"level_factor"` // real / expected since a persistent change (1 = none)
	ExtraKWhSoFar    float64            `json:"extra_kwh_so_far"`
	EpisodeZ         float64            `json:"episode_z"`
	VariableZ        map[string]float64 `json:"variable_z"`
	Base             Electrical         `json:"base_electrical"`
	Cur              Electrical         `json:"current_electrical"`
}

// FlaggedReading is a reading that failed a plausibility rule.
type FlaggedReading struct {
	Timestamp time.Time `json:"timestamp"`
	Flags     []string  `json:"flags"`
}

// Data-quality flags stored per reading.
const (
	FlagRange   = "DQ_RANGE"   // PF outside [0,1] or 0 V with consumption
	FlagVoltage = "DQ_VOLTAGE" // voltage outside the plausible band
	FlagJump    = "DQ_JUMP"    // voltage step between consecutive hours
	FlagPF      = "DQ_PF_JUMP" // power factor jumps against its neighbours
	FlagPhysics = "DQ_PHYSICS" // kWh does not match V·I·PF (own and local relation)
)

// Electrical summarizes mean electrical values of a window.
type Electrical struct {
	KWhPerDay   float64 `json:"kwh_per_day"`
	VoltageV    float64 `json:"voltage_v"`
	CurrentA    float64 `json:"current_a"`
	PowerFactor float64 `json:"power_factor"`
}

// QualityIssue reports whether the meter's readings cannot be trusted.
func (s MeterStats) QualityIssue(c Config) bool {
	return s.InvalidReadings >= c.MinInvalid || s.Coherence < c.MinCoherence || s.MissingHours+s.DuplicateHours >= c.MinInvalid
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
	if len(rs) == 0 {
		return s
	}
	// One reading per hour: repeated hours are counted and dropped (summing them
	// would double the consumption), and gaps are counted as missing hours.
	uniq := make([]domain.Reading, 0, len(rs))
	for _, r := range rs {
		if len(uniq) > 0 && r.Timestamp.Truncate(time.Hour).Equal(uniq[len(uniq)-1].Timestamp.Truncate(time.Hour)) {
			// An exact copy (a file loaded twice) is harmless; two different
			// readings for the same hour are a data problem.
			p := uniq[len(uniq)-1]
			if p.ConsumptionKWh != r.ConsumptionKWh || p.VoltageV != r.VoltageV || p.CurrentA != r.CurrentA || p.PowerFactor != r.PowerFactor {
				s.DuplicateHours++
			}
			continue
		}
		uniq = append(uniq, r)
	}
	rs = uniq
	n := len(rs)
	if span := int(rs[n-1].Timestamp.Sub(rs[0].Timestamp).Hours()) + 1; span > n {
		s.MissingHours = span - n
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
	_ = baseVolt
	for i, r := range rs {
		vAnom := !hard[i] && (r.VoltageV < c.VoltageMin || r.VoltageV > c.VoltageMax)
		vJump := i > 0 && !hard[i] && !hard[i-1] && math.Abs(r.VoltageV-rs[i-1].VoltageV) > c.VoltageJump
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
		if vJump {
			s.VoltageJumps++
		}
		if pfJ {
			s.PFJumps++
		}
		if incoh {
			s.Incoherent++
		}
		sus[i] = hard[i] || vAnom || vJump || pfJ || incoh
		if sus[i] {
			var flags []string
			for _, f := range []struct {
				on   bool
				name string
			}{{hard[i], FlagRange}, {vAnom, FlagVoltage}, {vJump, FlagJump}, {pfJ, FlagPF}, {incoh, FlagPhysics}} {
				if f.on {
					flags = append(flags, f.name)
				}
			}
			s.Flagged = append(s.Flagged, FlaggedReading{Timestamp: r.Timestamp, Flags: flags})
			s.InvalidReadings++
			if first < 0 {
				first = i
			}
			last = i
			if vAnom || vJump {
				vr[0], vr[1] = math.Min(vr[0], r.VoltageV), math.Max(vr[1], r.VoltageV)
			}
			if pfJ {
				pr[0], pr[1] = math.Min(pr[0], r.PowerFactor), math.Max(pr[1], r.PowerFactor)
			}
		}
	}
	if checked > 0 {
		s.Coherence = round(float64(coherent)/float64(checked), 3)
	}
	s.KFactor = round(baseRatio, 4)
	s.KMAD = round(mad(baseRatios, baseRatio), 4)
	// Worst 24 h window: share of flagged readings (decides DATA_QUALITY severity).
	for i := range rs {
		k, tot := 0, 0
		for j := i; j < n && rs[j].Timestamp.Sub(rs[i].Timestamp) < 24*time.Hour; j++ {
			tot++
			if sus[j] {
				k++
			}
		}
		if tot > 0 {
			s.MaxFlagShare24h = math.Max(s.MaxFlagShare24h, float64(k)/float64(tot))
		}
	}
	s.MaxFlagShare24h = round(s.MaxFlagShare24h, 3)
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
		if !math.IsInf(vr[0], 1) {
			s.VoltageRange = [2]float64{round(vr[0], 1), round(vr[1], 1)}
		}
		if !math.IsInf(pr[0], 1) {
			s.PFRange = [2]float64{round(pr[0], 2), round(pr[1], 2)}
		}
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
		s.HourlyP10[h] = round(percentile(perHourBase[h], 0.10), 3)
		s.HourlyP90[h] = round(percentile(perHourBase[h], 0.90), 3)
		expectedDay += medians[h]
	}
	floor := 0.1 * expectedDay / 24
	expectedAt := func(h int) float64 { return math.Max(medians[h], floor) }

	// --- 3. current state: last 24 hours ----------------------------------------
	lastTS := rs[n-1].Timestamp
	cur24 := func(i int) bool { return lastTS.Sub(rs[i].Timestamp) < 24*time.Hour }
	var curCount [24]int
	curN := 0
	for i, r := range rs {
		if cur24(i) {
			curN++
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
	if curN > 0 && curN < 24 {
		// Missing hours in the last 24 h: compare like with like instead of
		// reporting a drop that is only missing data.
		s.CurrentKWh *= 24 / float64(curN)
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

	// Level, energy and robust z of the change (feed projection, impact and confidence).
	s.LevelFactor = 1
	s.VariableZ = map[string]float64{}
	if ep := s.Episode; ep != nil {
		var ratios, zs []float64
		for i, r := range rs {
			if sus[i] || !inWindow(i) {
				continue
			}
			h := r.Timestamp.Hour()
			ratios = append(ratios, r.ConsumptionKWh/expectedAt(h))
			zs = append(zs, robustZ(r.ConsumptionKWh, medians[h], mads[h]))
			s.ExtraKWhSoFar += r.ConsumptionKWh - expectedAt(h)
		}
		if !ep.Recovered && len(ratios) > 0 {
			s.LevelFactor = round(median(ratios), 3)
		}
		s.ExtraKWhSoFar = round(s.ExtraKWhSoFar, 1)
		kz := median(zs)
		s.EpisodeZ = round(math.Abs(kz), 2)
		s.VariableZ["kwh"] = round(kz, 2)
		zOf := func(base, cur []float64) float64 {
			if len(base) == 0 || len(cur) == 0 {
				return 0
			}
			m := median(base)
			return round(robustZ(median(cur), m, mad(base, m)), 2)
		}
		s.VariableZ["current"] = zOf(bi, ci)
		s.VariableZ["voltage"] = zOf(bv, cv)
		s.VariableZ["power_factor"] = zOf(bpf, cpf)
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
	case s.QualityIssue(e.cfg) && s.InvalidReadings < e.cfg.MinInvalid:
		return domain.StatusAlert, fmt.Sprintf("%d horas faltantes y %d repetidas", s.MissingHours, s.DuplicateHours)
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
	a.ConfidenceBreakdown = e.confidenceBreakdown(a, s)
	a.Confidence = confidenceOf(a.ConfidenceBreakdown)
	a.Impact = e.impact(a, s)
	if ep := s.Episode; ep != nil && a.Type != domain.DataQuality {
		t := ep.Onset
		a.ChangePointAt = &t
		if ep.Recovered {
			end := ep.End.Add(time.Hour)
			a.EndedAt = &end
		}
	} else if s.IssueOnset != nil {
		t := *s.IssueOnset
		a.ChangePointAt = &t
	}
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
	if s.MaxFlagShare24h > e.cfg.DQHighShare || s.Coherence < e.cfg.MinCoherence {
		a.Severity = domain.SeverityHigh
	}
	if s.IssueOnset != nil {
		t := *s.IssueOnset
		ev.Onset = &t
		ev.OnsetDay = dayIndex(s, t)
	}
	ev.PersistentHours = s.IssueHours
	if s.VoltageAnomalies > 0 {
		ev.Signals = append(ev.Signals, sig("VOLTAGE_JUMPS", fmt.Sprintf("%d lecturas con voltaje fuera de %s–%s V (%s–%s V)", s.VoltageAnomalies, fmtNum(e.cfg.VoltageMin, 0), fmtNum(e.cfg.VoltageMax, 0), fmtNum(s.VoltageRange[0], 0), fmtNum(s.VoltageRange[1], 0)), float64(s.VoltageAnomalies)))
	}
	if s.VoltageJumps > 0 {
		ev.Signals = append(ev.Signals, sig("VOLTAGE_STEPS", fmt.Sprintf("%d saltos de voltaje de más de %s V entre horas seguidas", s.VoltageJumps, fmtNum(e.cfg.VoltageJump, 0)), float64(s.VoltageJumps)))
	}
	if s.PFJumps > 0 {
		ev.Signals = append(ev.Signals, sig("PF_JUMPS", fmt.Sprintf("%d saltos bruscos del factor de potencia (%s–%s)", s.PFJumps, fmtNum(s.PFRange[0], 2), fmtNum(s.PFRange[1], 2)), float64(s.PFJumps)))
	}
	if s.Incoherent > 0 {
		ev.Signals = append(ev.Signals, sig("INCOHERENT_READINGS", fmt.Sprintf("%d lecturas donde el consumo no cuadra con V·I·PF (k habitual %s)", s.Incoherent, fmtNum(s.KFactor, 2)), float64(s.Incoherent)))
	}
	if s.PFOutOfRange > 0 {
		ev.Signals = append(ev.Signals, sig("PF_OUT_OF_RANGE", fmt.Sprintf("%d lecturas con factor de potencia fuera de [0, 1]", s.PFOutOfRange), float64(s.PFOutOfRange)))
	}
	if s.ZeroVoltage > 0 {
		ev.Signals = append(ev.Signals, sig("ZERO_VOLTAGE", fmt.Sprintf("%d horas con 0 V y consumo positivo", s.ZeroVoltage), float64(s.ZeroVoltage)))
	}
	if s.MissingHours > 0 {
		ev.Signals = append(ev.Signals, sig("MISSING_HOURS", fmt.Sprintf("%d horas sin lectura", s.MissingHours), float64(s.MissingHours)))
	}
	if s.DuplicateHours > 0 {
		ev.Signals = append(ev.Signals, sig("DUPLICATE_READINGS", fmt.Sprintf("%d lecturas repetidas para la misma hora", s.DuplicateHours), float64(s.DuplicateHours)))
	}
	if math.Abs(s.VariationPct) < e.cfg.ShiftThreshold*100 && s.Episode == nil {
		ev.Signals = append(ev.Signals, sig("STABLE_CONSUMPTION", fmt.Sprintf("Consumo estable (%s): no es un cambio de carga", fmtPct(s.VariationPct)), s.VariationPct))
	}
	ev.Signals = append(ev.Signals, sig("FLAG_SHARE", fmt.Sprintf("Hasta %s de las lecturas de 24 h marcadas como inconsistentes", fmtPctPlain(s.MaxFlagShare24h*100)), round(s.MaxFlagShare24h*100, 1)))
	// Events only corroborate: the verdict above never depends on them.
	for _, x := range events {
		if x.Category() == domain.EventInformational {
			ev.RelatedEvents = append(ev.RelatedEvents, x)
			ev.Signals = append(ev.Signals, sig("DQ_EVENT", fmt.Sprintf("Registro operativo que lo corrobora: «%s» (%s)", x.Description, x.Timestamp.Format("02/01 15:04")), 1))
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
	related, coherent, why := e.matchEvents(a.MeterID, ep, events)
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
		if ep.Recovered {
			a.Type, a.Severity = domain.FalsePositive, domain.SeverityLow
			ev.Signals = append(ev.Signals, sig("RECOVERED", fmt.Sprintf("El consumo volvió a su nivel al terminar el evento (%s), tras %d h", ep.End.Add(time.Hour).Format("02/01 15:04"), ep.Hours), float64(ep.Hours)))
		} else {
			a.Type, a.Severity = domain.ExplainableAnomaly, domain.SeverityMedium
		}
		if !elec {
			ev.Signals = append(ev.Signals, sig("HEALTHY_ELECTRICAL", fmt.Sprintf("Relación eléctrica sana: la corriente acompaña al consumo y el factor de potencia se mantiene en %s", fmtNum(s.Cur.PowerFactor, 2)), s.Cur.PowerFactor))
		}
		return
	}

	a.Type, a.Severity = domain.RealAnomaly, domain.SeverityMedium
	iChg := pctChange(s.Base.CurrentA, s.Cur.CurrentA)
	if math.Abs(ep.MeanDevPct) >= e.cfg.HighPct || ep.Hours > 12 || elec || s.Cur.PowerFactor < 0.85 || iChg > 50 {
		a.Severity = domain.SeverityHigh
	}
	if len(related) > 0 {
		ev.Signals = append(ev.Signals, sig("NO_EVENT", fmt.Sprintf("Ningún evento explica el cambio: %s", why), 0))
	} else {
		ev.Signals = append(ev.Signals, sig("NO_EVENT", fmt.Sprintf("Ningún evento operativo en ±%d h del inicio del cambio", int(e.cfg.EventWindow.Hours())), 0))
	}
	if math.Abs(iChg) > 25 {
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

// matchEvents evaluates the meter's events against the change. An event
// explains it only if it is EXPLANATORY, sits within ±EventWindow of the change
// point, its expected effect matches the direction and, for outages that state
// a duration, the observed duration matches within DurationTol. It returns the
// events it evaluated, the one that explains (if any) and why the rest do not.
func (e *Engine) matchEvents(meterID string, ep *Episode, events []domain.Event) ([]domain.Event, *domain.Event, string) {
	var related []domain.Event
	var coherent *domain.Event
	var reasons []string
	window := e.cfg.EventWindow
	for _, ev := range events {
		if ev.MeterID != meterID {
			continue
		}
		diff := ev.Timestamp.Sub(ep.Onset)
		if diff < 0 {
			diff = -diff
		}
		// Events far from the change are not related; UNKNOWN records near it are
		// kept as evidence that nothing operational was reported.
		if diff > window && diff > 24*time.Hour {
			continue
		}
		related = append(related, ev)
		dir := domain.EffectUp
		if ep.Direction < 0 {
			dir = domain.EffectDown
		}
		switch {
		case ev.Category() != domain.EventExplanatory:
			reasons = append(reasons, fmt.Sprintf("«%s» (%s) no describe una causa operativa", ev.Description, ev.Type))
		case diff > window:
			reasons = append(reasons, fmt.Sprintf("«%s» ocurre a %s h del cambio", ev.Description, fmtNum(diff.Hours(), 0)))
		case ev.ExpectedEffect() != dir:
			reasons = append(reasons, fmt.Sprintf("«%s» va en la dirección contraria", ev.Description))
		case ev.Kind() == domain.EventShutdown && ev.DurationHours() > 0 && durationMismatch(ev.DurationHours(), ep, e.cfg.DurationTol):
			reasons = append(reasons, fmt.Sprintf("«%s» dura %s h y el cambio %d h", ev.Description, fmtNum(ev.DurationHours(), 0), ep.Hours))
		default:
			if coherent == nil {
				c := ev
				coherent = &c
			}
		}
	}
	if coherent != nil {
		// The event that explains the change goes first: it is the one the
		// explanation cites.
		ordered := []domain.Event{*coherent}
		for _, ev := range related {
			if ev.ID != coherent.ID || ev.Timestamp != coherent.Timestamp {
				ordered = append(ordered, ev)
			}
		}
		related = ordered
	}
	return related, coherent, joinReasons(reasons)
}

// durationMismatch compares an announced outage duration with the episode. A finished
// episode must last the announced time ± tol; one still in progress is compatible
// until it exceeds it (seen hour by hour, a 12 h outage is 6 h long at hour 6).
func durationMismatch(announced float64, ep *Episode, tol float64) bool {
	if !ep.Recovered {
		return float64(ep.Hours) > announced+tol
	}
	return math.Abs(announced-float64(ep.Hours)) > tol
}

func joinReasons(rs []string) string {
	switch len(rs) {
	case 0:
		return ""
	case 1:
		return rs[0]
	}
	return fmt.Sprintf("%s; %s", rs[0], joinReasons(rs[1:]))
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

// confidenceBreakdown computes the four weighted components of the plan:
// magnitude 0.35 · persistence 0.25 · agreeing variables 0.25 · event coherence 0.15.
func (e *Engine) confidenceBreakdown(a domain.Anomaly, s MeterStats) []domain.ConfidenceComponent {
	ev := a.Evidence
	var mag, pers, vars, evt float64
	var magD, varsD, evtD string
	agree := func(conds ...bool) float64 {
		n := 0
		for _, c := range conds {
			if c {
				n++
			}
		}
		return float64(n) / float64(len(conds))
	}
	z := s.VariableZ
	switch a.Type {
	case domain.DataQuality:
		mag = clamp(s.MaxFlagShare24h/0.3, 0, 1)
		magD = fmt.Sprintf("%s de lecturas marcadas en 24 h", fmtPctPlain(s.MaxFlagShare24h*100))
		pers = clamp(float64(s.IssueHours)/e.cfg.PersistenceHours, 0, 1)
		vars = agree(s.VoltageAnomalies+s.VoltageJumps > 0, s.PFJumps > 0, s.Incoherent > 0, math.Abs(s.VariationPct) < e.cfg.ShiftThreshold*100)
		varsD = "voltaje, FP, relación V·I·PF y consumo estable"
		evt, evtD = 0.5, "sin registro que lo corrobore"
		for _, x := range ev.RelatedEvents {
			if x.Category() == domain.EventInformational {
				evt, evtD = 1, "un registro de calidad de datos lo corrobora"
			}
		}
	default:
		mag = clamp(s.EpisodeZ/10, 0, 1)
		magD = fmt.Sprintf("|z| robusto %s", fmtNum(s.EpisodeZ, 1))
		pers = clamp(float64(ev.PersistentHours)/e.cfg.PersistenceHours, 0, 1)
		up := s.Episode != nil && s.Episode.Direction > 0
		switch a.Type {
		case domain.RealAnomaly:
			vars = agree(z["kwh"] > 3, z["current"] > 3, z["power_factor"] < -3)
			varsD = "consumo ↑, corriente ↑, FP ↓"
		case domain.ExplainableAnomaly:
			vars = agree(sign(z["kwh"]) == dir(up) && math.Abs(z["kwh"]) > 3, sign(z["current"]) == dir(up) && math.Abs(z["current"]) > 3)
			varsD = "consumo y corriente en la dirección del evento"
		default:
			vars = agree(z["kwh"] < -3, z["current"] < -3)
			varsD = "consumo ↓ y corriente ↓ durante el evento"
		}
		if a.Type == domain.RealAnomaly {
			evt, evtD = 1, "se confirmó que ningún evento lo explica"
		} else if ev.EventExplainsShift {
			evt, evtD = 1, "evento coherente en hora, dirección y duración"
		}
	}
	c := func(key, label string, w, sc float64, d string) domain.ConfidenceComponent {
		return domain.ConfidenceComponent{Key: key, Label: label, Weight: w, Score: round(sc, 2), Detail: d}
	}
	return []domain.ConfidenceComponent{
		c("magnitude", "Magnitud", 0.35, mag, magD),
		c("persistence", "Persistencia", 0.25, pers, fmt.Sprintf("%d h", max(ev.PersistentHours, 0))),
		c("variables", "Variables que coinciden", 0.25, vars, varsD),
		c("events", "Coherencia con eventos", 0.15, evt, evtD),
	}
}

func sign(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func dir(up bool) int {
	if up {
		return 1
	}
	return -1
}

func confidenceOf(cs []domain.ConfidenceComponent) float64 {
	t := 0.0
	for _, c := range cs {
		t += c.Weight * c.Score
	}
	return round(clamp(t, 0, 0.97), 2)
}

// impact quantifies the anomaly: extra energy, monthly cost and reactive energy.
func (e *Engine) impact(a domain.Anomaly, s MeterStats) *domain.Impact {
	extraDay := s.CurrentKWh - s.BaselineKWh
	pf := s.Cur.PowerFactor
	if pf <= 0 || pf > 1 {
		pf = s.Base.PowerFactor
	}
	ratio := 0.0
	if pf > 0 && pf <= 1 {
		ratio = math.Tan(math.Acos(pf))
	}
	im := &domain.Impact{
		ExtraKWhPerDay:   round(extraDay, 1),
		ExtraKWhPerMonth: round(extraDay*30, 0),
		ExtraKWhSoFar:    s.ExtraKWhSoFar,
		CostPerMonthCOP:  round(extraDay*30*e.cfg.TariffCOPPerKWh, 0),
		TariffCOPPerKWh:  e.cfg.TariffCOPPerKWh,
		PowerFactor:      round(pf, 2),
		ReactiveRatio:    round(ratio, 2),
		ReactiveExcess:   round(math.Max(0, ratio-e.cfg.ReactiveLimit), 2),
		ReactiveKVArhDay: round(s.CurrentKWh*ratio, 0),
	}
	switch a.Type {
	case domain.DataQuality:
		// No extra energy, but the meter's data is unusable for billing and reports.
		im.Normalized = round(clamp(0.5+s.MaxFlagShare24h, 0, 1), 3)
	default:
		if s.BaselineKWh > 0 {
			im.Normalized = round(clamp(math.Abs(extraDay)/s.BaselineKWh, 0, 1), 3)
		}
	}
	return im
}

// Prioritize computes the priority score and rank of each finding:
//
//	severity weight (HIGH 3 · MEDIUM 2 · LOW 1) × confidence × normalized impact
func Prioritize(fs []Finding) {
	for i := range fs {
		a := &fs[i].Anomaly
		w := map[domain.Severity]float64{domain.SeverityHigh: 3, domain.SeverityMedium: 2, domain.SeverityLow: 1}[a.Severity]
		imp := 0.0
		if a.Impact != nil {
			imp = a.Impact.Normalized
		}
		a.PriorityScore = round(w*a.Confidence*imp, 3)
	}
	// Ties are broken explicitly so the order never depends on the input order:
	// a real anomaly before a data issue, then the larger energy impact, then the id.
	typeRank := map[domain.AnomalyType]int{domain.RealAnomaly: 0, domain.DataQuality: 1, domain.ExplainableAnomaly: 2, domain.FalsePositive: 3}
	extra := func(a domain.Anomaly) float64 {
		if a.Impact == nil {
			return 0
		}
		return a.Impact.ExtraKWhPerDay
	}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i].Anomaly, fs[j].Anomaly
		switch {
		case a.PriorityScore != b.PriorityScore:
			return a.PriorityScore > b.PriorityScore
		case typeRank[a.Type] != typeRank[b.Type]:
			return typeRank[a.Type] < typeRank[b.Type]
		case extra(a) != extra(b):
			return extra(a) > extra(b)
		default:
			return a.MeterID < b.MeterID
		}
	})
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
