package explain

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

var numberRe = regexp.MustCompile(`\d+(?:[.,]\d+)*`)

// parseNumber understands Spanish (1.070,5) and plain (1070.5) formats.
func parseNumber(s string) (float64, bool) {
	switch {
	case strings.Contains(s, ","):
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	case strings.Count(s, ".") > 1 || (strings.Contains(s, ".") && len(s)-strings.LastIndex(s, ".") == 4):
		// "1.070" or "4.032": dots are thousands separators
		s = strings.ReplaceAll(s, ".", "")
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// allowedNumbers collects every number that may legitimately appear in an
// explanation of this anomaly: the evidence, the figures derived from it that
// are sent to the model (extra kWh, impact) and the confidence.
func allowedNumbers(a domain.Anomaly) []float64 {
	ev := a.Evidence
	add := func(out []float64, vs ...float64) []float64 {
		for _, v := range vs {
			out = append(out, math.Abs(v))
		}
		return out
	}
	var out []float64
	out = add(out, ev.BaselineKWh, ev.CurrentKWh, ev.VariationPct, float64(ev.OnsetDay), float64(ev.EndDay),
		float64(ev.PersistentHours), ev.NightRatio, float64(ev.InvalidReadings), ev.PhysicalCoherence*100, (1-ev.PhysicalCoherence)*100,
		ev.ShiftPct, ev.BaselineKWh/24, ev.CurrentKWh/24, 24, 7, 14,
		ev.CurrentKWh-ev.BaselineKWh, a.Confidence, a.Confidence*100)
	if im := a.Impact; im != nil {
		out = add(out, im.ExtraKWhPerDay, im.ExtraKWhPerMonth, im.ExtraKWhSoFar, im.CostPerMonthCOP, im.CostPerMonthCOP/1e6,
			im.TariffCOPPerKWh, im.PowerFactor, im.ReactiveRatio, im.ReactiveExcess, im.ReactiveKVArhDay, 30, 0.5)
	}
	if ev.Onset != nil {
		out = add(out, float64(ev.Onset.Day()), float64(ev.Onset.Hour()))
	}
	for _, s := range ev.Signals {
		out = add(out, s.Value)
		for _, m := range numberRe.FindAllString(s.Description, -1) {
			if v, ok := parseNumber(m); ok {
				out = append(out, v)
			}
		}
	}
	for _, v := range ev.Variables {
		out = add(out, v.Baseline, v.Current, v.DeltaPct, v.Current-v.Baseline)
	}
	for _, e := range ev.RelatedEvents {
		out = add(out, float64(e.Timestamp.Day()), float64(e.Timestamp.Hour()), float64(e.Timestamp.Month()))
		for _, m := range numberRe.FindAllString(e.Description, -1) {
			if v, ok := parseNumber(m); ok {
				out = append(out, v)
			}
		}
	}
	if id := numberRe.FindString(ev.MeterID); id != "" {
		if v, ok := parseNumber(id); ok {
			out = append(out, v)
		}
	}
	return out
}

func grounded(v float64, allowed []float64) bool {
	if v <= 3 { // ordinals and small counts ("2 pasos", "1 medidor")
		return true
	}
	for _, a := range allowed {
		// Relative tolerance for rounding, and ±0,5 so "555" matches 555,3.
		tol := math.Max(0.5, math.Abs(a)*0.015)
		if v != math.Trunc(v) || math.Abs(a) < 10 {
			tol = math.Max(0.06, math.Abs(a)*0.015)
		}
		if math.Abs(v-a) <= tol {
			return true
		}
	}
	return false
}

// Validate rejects explanations that cite numbers not present in the evidence.
func Validate(e Explanation, a domain.Anomaly) error {
	if strings.TrimSpace(e.Reason) == "" || strings.TrimSpace(e.RecommendedAction) == "" {
		return fmt.Errorf("empty reason or action")
	}
	allowed := allowedNumbers(a)
	texts := append([]string{e.Reason, e.RecommendedAction, e.EvidenceSummary}, e.NextSteps...)
	for _, t := range texts {
		for _, m := range numberRe.FindAllString(t, -1) {
			v, ok := parseNumber(m)
			if !ok || !grounded(v, allowed) {
				return fmt.Errorf("%w: %q", ErrUngrounded, m)
			}
		}
	}
	return nil
}
