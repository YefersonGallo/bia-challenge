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
	ShiftThreshold   float64       // daily deviation (fraction) that counts as a shift
	CriticalPct      float64       // variation that marks a meter as CRITICAL
	HighPct          float64       // variation above which a real anomaly is HIGH
	ZThreshold       float64       // robust z-score for spikes
	EventWindow      time.Duration // how close an event must be to the onset
	CoherenceTol     float64       // tolerance of kWh vs V·I·PF
	MinInvalid       int           // invalid readings that trigger DATA_QUALITY
	MinCoherence     float64       // physical coherence below which data is suspect
	PFDropThreshold  float64       // power-factor drop considered anomalous
	NightRatioSignal float64       // night consumption ratio worth reporting
}

// DefaultConfig returns the thresholds documented in the README.
func DefaultConfig() Config {
	return Config{
		BaselineDays:     7,
		ShiftThreshold:   0.25,
		CriticalPct:      100,
		HighPct:          50,
		ZThreshold:       3.5,
		EventWindow:      24 * time.Hour,
		CoherenceTol:     0.10,
		MinInvalid:       5,
		MinCoherence:     0.90,
		PFDropThreshold:  0.05,
		NightRatioSignal: 1.5,
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

// Episode is a sustained deviation from the baseline.
type Episode struct {
	OnsetDay   int       `json:"onset_day"`
	EndDay     int       `json:"end_day"`
	Onset      time.Time `json:"onset"`
	Direction  int       `json:"direction"` // +1 increase, -1 decrease
	Hours      int       `json:"hours"`
	Recovered  bool      `json:"recovered"`
	MeanDevPct float64   `json:"mean_deviation_pct"`
}

// MeterStats is the rule layer: statistics available without any AI.
type MeterStats struct {
	MeterID         string             `json:"meter_id"`
	BaselineKWh     float64            `json:"baseline_kwh"`
	CurrentKWh      float64            `json:"current_kwh"`
	VariationPct    float64            `json:"variation_pct"`
	Status          domain.MeterStatus `json:"status"`
	StatusReason    string             `json:"status_reason"`
	InvalidReadings int                `json:"invalid_readings"`
	PFOutOfRange    int                `json:"pf_out_of_range"`
	ZeroVoltage     int                `json:"zero_voltage"`
	Coherence       float64            `json:"physical_coherence"`
	Days            []DayPoint         `json:"days"`
	HourlyBaseline  [24]float64        `json:"hourly_baseline"`
	HourlyCurrent   [24]float64        `json:"hourly_current"`
	NightRatio      float64            `json:"night_ratio"`
	Spikes          int                `json:"spikes"`
	Episode         *Episode           `json:"episode,omitempty"`
	Base            Electrical         `json:"base_electrical"`
	Cur             Electrical         `json:"current_electrical"`
	InvalidDaysCur  int                `json:"invalid_days_current"`
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
		res.Stats[m.ID] = e.ComputeStats(byMeter[m.ID])
		res.Stats[m.ID] = withID(res.Stats[m.ID], m.ID)
		res.Invalid += res.Stats[m.ID].InvalidReadings
	}
	obs("readings", fmt.Sprintf("%s lecturas · %d inválidas", fmtInt(len(in.Readings)), res.Invalid))

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
		if e.electricalAnomaly(s) || s.QualityIssue(e.cfg) {
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

func withID(s MeterStats, id string) MeterStats { s.MeterID = id; return s }

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

// ComputeStats builds the rule-layer statistics of one meter (readings sorted).
func (e *Engine) ComputeStats(rs []domain.Reading) MeterStats {
	s := MeterStats{}
	if len(rs) == 0 {
		s.Status = domain.StatusOK
		return s
	}
	start := rs[0].Timestamp.Truncate(24 * time.Hour)
	baseDays := e.cfg.BaselineDays

	type bucket struct{ kwh, v, i, pf []float64 }
	perHourBase := make([][]float64, 24)
	perHourCur := make([][]float64, 24)
	days := map[int]*DayPoint{}
	dayElec := map[int]*bucket{}
	base, cur := &bucket{}, &bucket{}
	checked, coherent := 0, 0
	invalidDays := map[int]bool{}

	for _, r := range rs {
		day := int(r.Timestamp.Sub(start).Hours())/24 + 1
		hod := r.Timestamp.Hour()
		dp := days[day]
		if dp == nil {
			dp = &DayPoint{Day: day, Date: start.AddDate(0, 0, day-1).Format("2006-01-02")}
			days[day] = dp
			dayElec[day] = &bucket{}
		}
		dp.KWh += r.ConsumptionKWh
		pfBad, zeroV := e.invalid(r)
		if pfBad || zeroV {
			dp.Invalid++
			s.InvalidReadings++
			if pfBad {
				s.PFOutOfRange++
			}
			if zeroV {
				s.ZeroVoltage++
			}
			if day > baseDays {
				invalidDays[day] = true
			}
		} else {
			// physical coherence: hourly kWh ≈ V·I·PF / 1000
			if r.VoltageV > 0 && r.CurrentA > 0 && r.PowerFactor > 0 {
				checked++
				expected := r.VoltageV * r.CurrentA * r.PowerFactor / 1000
				if math.Abs(r.ConsumptionKWh/expected-1) <= e.cfg.CoherenceTol {
					coherent++
				}
			}
			w := cur
			if day <= baseDays {
				w = base
			}
			w.v = append(w.v, r.VoltageV)
			w.i = append(w.i, r.CurrentA)
			w.pf = append(w.pf, r.PowerFactor)
			de := dayElec[day]
			de.v = append(de.v, r.VoltageV)
			de.i = append(de.i, r.CurrentA)
			de.pf = append(de.pf, r.PowerFactor)
		}
		if day <= baseDays {
			perHourBase[hod] = append(perHourBase[hod], r.ConsumptionKWh)
			s.BaselineKWh += r.ConsumptionKWh
		} else {
			perHourCur[hod] = append(perHourCur[hod], r.ConsumptionKWh)
			s.CurrentKWh += r.ConsumptionKWh
		}
	}
	if checked > 0 {
		s.Coherence = float64(coherent) / float64(checked)
	} else {
		s.Coherence = 1
	}
	s.InvalidDaysCur = len(invalidDays)

	// hour-of-day baseline (median + MAD)
	medians, mads := [24]float64{}, [24]float64{}
	expectedDay := 0.0
	for h := 0; h < 24; h++ {
		medians[h] = median(perHourBase[h])
		mads[h] = mad(perHourBase[h], medians[h])
		s.HourlyBaseline[h] = round(medians[h], 3)
		s.HourlyCurrent[h] = round(median(perHourCur[h]), 3)
		expectedDay += medians[h]
	}

	// spikes: isolated hours far from their hour-of-day baseline
	for idx, r := range rs {
		day := int(r.Timestamp.Sub(start).Hours())/24 + 1
		if day <= baseDays {
			continue
		}
		h := r.Timestamp.Hour()
		z := robustZ(r.ConsumptionKWh, medians[h], mads[h])
		if math.Abs(z) < e.cfg.ZThreshold {
			continue
		}
		prevOut := idx > 0 && math.Abs(robustZ(rs[idx-1].ConsumptionKWh, medians[rs[idx-1].Timestamp.Hour()], mads[rs[idx-1].Timestamp.Hour()])) >= e.cfg.ZThreshold
		nextOut := idx+1 < len(rs) && math.Abs(robustZ(rs[idx+1].ConsumptionKWh, medians[rs[idx+1].Timestamp.Hour()], mads[rs[idx+1].Timestamp.Hour()])) >= e.cfg.ZThreshold
		if !prevOut && !nextOut {
			s.Spikes++
		}
	}

	// daily points
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
		de := dayElec[d]
		dp.VoltageV, dp.CurrentA, dp.PowerFactor = round(mean(de.v), 1), round(mean(de.i), 2), round(mean(de.pf), 3)
		s.Days = append(s.Days, *dp)
	}

	baseDaysN := math.Max(1, float64(min(baseDays, len(keys))))
	curDaysN := math.Max(1, float64(len(keys)-baseDays))
	s.Base = Electrical{KWhPerDay: round(s.BaselineKWh/baseDaysN, 2), VoltageV: round(mean(base.v), 1), CurrentA: round(mean(base.i), 2), PowerFactor: round(mean(base.pf), 3)}
	s.Cur = Electrical{KWhPerDay: round(s.CurrentKWh/curDaysN, 2), VoltageV: round(mean(cur.v), 1), CurrentA: round(mean(cur.i), 2), PowerFactor: round(mean(cur.pf), 3)}
	s.VariationPct = round(pctChange(s.BaselineKWh, s.CurrentKWh), 1)
	s.BaselineKWh, s.CurrentKWh = round(s.BaselineKWh, 1), round(s.CurrentKWh, 1)

	// night pattern (22:00–05:59)
	nb, nc := []float64{}, []float64{}
	for _, h := range []int{22, 23, 0, 1, 2, 3, 4, 5} {
		nb = append(nb, medians[h])
		nc = append(nc, median(perHourCur[h]))
	}
	if mean(nb) > 0 {
		s.NightRatio = round(mean(nc)/mean(nb), 2)
	}

	s.Episode = e.findEpisode(s.Days, start)
	s.Status, s.StatusReason = e.status(s)
	return s
}

// findEpisode looks for the first sustained deviation in the analysis window.
func (e *Engine) findEpisode(days []DayPoint, start time.Time) *Episode {
	th := e.cfg.ShiftThreshold * 100
	for i, d := range days {
		if d.Day <= e.cfg.BaselineDays || math.Abs(d.DeviationPct) < th {
			continue
		}
		dir := 1
		if d.DeviationPct < 0 {
			dir = -1
		}
		end := i
		devs := []float64{d.DeviationPct}
		for j := i + 1; j < len(days); j++ {
			if float64(dir)*days[j].DeviationPct >= th {
				end = j
				devs = append(devs, days[j].DeviationPct)
				continue
			}
			break
		}
		return &Episode{
			OnsetDay: d.Day, EndDay: days[end].Day, Onset: start.AddDate(0, 0, d.Day-1), Direction: dir,
			Hours: (days[end].Day - d.Day + 1) * 24, Recovered: end < len(days)-1, MeanDevPct: round(mean(devs), 1),
		}
	}
	return nil
}

func (e *Engine) status(s MeterStats) (domain.MeterStatus, string) {
	switch {
	case s.VariationPct >= e.cfg.CriticalPct:
		return domain.StatusCritical, fmt.Sprintf("variación %s sobre el baseline", fmtPct(s.VariationPct))
	case s.QualityIssue(e.cfg):
		return domain.StatusAlert, fmt.Sprintf("%d lecturas fuera de rango físico", s.InvalidReadings)
	case math.Abs(s.VariationPct) >= e.cfg.ShiftThreshold*100 || s.Episode != nil:
		return domain.StatusAlert, fmt.Sprintf("variación %s frente al baseline", fmtPct(s.VariationPct))
	default:
		return domain.StatusOK, "dentro del rango esperado"
	}
}

func (e *Engine) electricalAnomaly(s MeterStats) bool {
	pfDrop := s.Base.PowerFactor - s.Cur.PowerFactor
	iChg := pctChange(s.Base.CurrentA, s.Cur.CurrentA)
	kChg := pctChange(s.Base.KWhPerDay, s.Cur.KWhPerDay)
	return pfDrop >= e.cfg.PFDropThreshold || math.Abs(iChg-kChg) > 20
}

// Classify applies the decision tree: data quality → events → magnitude.
func (e *Engine) Classify(m domain.Meter, s MeterStats, events []domain.Event) *Finding {
	ev := domain.Evidence{
		MeterID: m.ID, MeterName: m.Name, BaselineKWh: s.BaselineKWh, CurrentKWh: s.CurrentKWh, VariationPct: s.VariationPct,
		NightRatio: s.NightRatio, InvalidReadings: s.InvalidReadings, PhysicalCoherence: round(s.Coherence, 3),
		Variables: e.variables(s),
	}
	a := domain.Anomaly{MeterID: m.ID, Status: domain.AnomalyOpen}

	switch {
	case s.QualityIssue(e.cfg):
		a.Type = domain.DataQuality
		a.Severity = domain.SeverityMedium
		if s.InvalidReadings >= 20 || s.Coherence < e.cfg.MinCoherence {
			a.Severity = domain.SeverityHigh
		}
		if s.PFOutOfRange > 0 {
			ev.Signals = append(ev.Signals, sig("PF_OUT_OF_RANGE", fmt.Sprintf("%d lecturas con factor de potencia mayor a 1", s.PFOutOfRange), float64(s.PFOutOfRange)))
		}
		if s.ZeroVoltage > 0 {
			ev.Signals = append(ev.Signals, sig("ZERO_VOLTAGE", fmt.Sprintf("%d horas con 0 V y consumo positivo", s.ZeroVoltage), float64(s.ZeroVoltage)))
		}
		if s.Coherence < e.cfg.MinCoherence {
			ev.Signals = append(ev.Signals, sig("INCOHERENT_READINGS", fmt.Sprintf("kWh no cuadra con V·I·PF en el %s de las lecturas", fmtPctPlain((1-s.Coherence)*100)), round((1-s.Coherence)*100, 1)))
		}
		if math.Abs(s.VariationPct) < e.cfg.ShiftThreshold*100 {
			ev.Signals = append(ev.Signals, sig("STABLE_CONSUMPTION", fmt.Sprintf("Consumo estable (%s): no es un cambio de carga", fmtPct(s.VariationPct)), s.VariationPct))
		}
		ev.PersistentHours = s.InvalidDaysCur * 24

	case s.Episode != nil:
		ep := s.Episode
		ev.OnsetDay, ev.EndDay, ev.PersistentHours = ep.OnsetDay, ep.EndDay, ep.Hours
		related, coherent := e.matchEvents(m.ID, ep, events)
		ev.RelatedEvents = related
		ev.Signals = append(ev.Signals, sig("PERSISTENT_SHIFT", fmt.Sprintf("%d h seguidas fuera de la banda (±%s) desde el día %d", ep.Hours, fmtPctPlain(e.cfg.ShiftThreshold*100), ep.OnsetDay), float64(ep.Hours)))
		if coherent != nil {
			ev.EventExplainsShift = true
			ev.Signals = append(ev.Signals, sig("EVENT_COHERENT", fmt.Sprintf("Evento coherente: %s (%s)", coherent.Description, coherent.Timestamp.Format("02/01 15:04")), 1))
			if coherent.Kind() == domain.EventShutdown {
				a.Type, a.Severity = domain.FalsePositive, domain.SeverityLow
				if ep.Recovered {
					ev.Signals = append(ev.Signals, sig("RECOVERED", fmt.Sprintf("Recuperación completa después del día %d", ep.EndDay), float64(ep.EndDay)))
				}
			} else {
				a.Type, a.Severity = domain.ExplainableAnomaly, domain.SeverityMedium
			}
			if !e.electricalAnomaly(s) {
				ev.Signals = append(ev.Signals, sig("HEALTHY_ELECTRICAL", fmt.Sprintf("Relación eléctrica sana: factor de potencia %s", fmtNum(s.Cur.PowerFactor, 2)), s.Cur.PowerFactor))
			}
		} else {
			a.Type, a.Severity = domain.RealAnomaly, domain.SeverityMedium
			elec := e.electricalAnomaly(s)
			if math.Abs(s.VariationPct) >= e.cfg.HighPct || elec {
				a.Severity = domain.SeverityHigh
			}
			ev.Signals = append(ev.Signals, sig("NO_EVENT", fmt.Sprintf("Ningún evento operativo en ±%d h del inicio del cambio", int(e.cfg.EventWindow.Hours())), 0))
			if s.NightRatio >= e.cfg.NightRatioSignal {
				ev.Signals = append(ev.Signals, sig("NIGHT_PATTERN", fmt.Sprintf("Consumo nocturno %s× el baseline", fmtNum(s.NightRatio, 1)), s.NightRatio))
			}
			if pfDrop := s.Base.PowerFactor - s.Cur.PowerFactor; pfDrop >= e.cfg.PFDropThreshold {
				ev.Signals = append(ev.Signals, sig("PF_DROP", fmt.Sprintf("Factor de potencia de %s a %s", fmtNum(s.Base.PowerFactor, 2), fmtNum(s.Cur.PowerFactor, 2)), round(pfDrop, 2)))
			}
			if iChg := pctChange(s.Base.CurrentA, s.Cur.CurrentA); iChg > 25 {
				ev.Signals = append(ev.Signals, sig("CURRENT_RISE", fmt.Sprintf("Corriente media %s", fmtPct(iChg)), round(iChg, 1)))
			}
		}
		if s.Spikes > 0 {
			ev.Signals = append(ev.Signals, sig("SPIKES", fmt.Sprintf("%d picos aislados (|z| > %s)", s.Spikes, fmtNum(e.cfg.ZThreshold, 1)), float64(s.Spikes)))
		}
	default:
		return nil
	}

	a.Evidence = ev
	a.Confidence = e.confidence(a, s)
	return &Finding{Anomaly: a, Stats: s}
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
		v("Consumo (7 días)", "kWh", s.BaselineKWh, s.CurrentKWh),
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
		strength = clamp(math.Abs(s.VariationPct)/100, 0, 1)
	case domain.DataQuality:
		strength = clamp(float64(s.InvalidReadings)/25+(1-s.Coherence)*2, 0, 1)
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
	for i := range fs {
		a := &fs[i].Anomaly
		s := fs[i].Stats
		mag := clamp(math.Abs(s.VariationPct)/100, 0, 1)
		pers := clamp(float64(a.Evidence.PersistentHours)/168, 0, 1)
		if a.Type == domain.DataQuality {
			mag = clamp(0.5+(1-s.Coherence)*2, 0, 1)
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
