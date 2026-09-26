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

// allowedNumbers collects every number that may legitimately appear in an
// explanation of this evidence.
func allowedNumbers(ev domain.Evidence) []float64 {
	add := func(out []float64, vs ...float64) []float64 {
		for _, v := range vs {
			out = append(out, math.Abs(v))
		}
		return out
	}
	var out []float64
	out = add(out, ev.BaselineKWh, ev.CurrentKWh, ev.VariationPct, float64(ev.OnsetDay), float64(ev.EndDay),
		float64(ev.PersistentHours), ev.NightRatio, float64(ev.InvalidReadings), ev.PhysicalCoherence*100, (1-ev.PhysicalCoherence)*100,
		ev.ShiftPct, ev.BaselineKWh/24, ev.CurrentKWh/24, 24, 7, 14)
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
		tol := math.Max(0.06, math.Abs(a)*0.015)
		if math.Abs(v-a) <= tol {
			return true
		}
	}
	return false
}

// Validate rejects explanations that cite numbers not present in the evidence.
func Validate(e Explanation, ev domain.Evidence) error {
	if strings.TrimSpace(e.Reason) == "" || strings.TrimSpace(e.RecommendedAction) == "" {
		return fmt.Errorf("empty reason or action")
	}
	allowed := allowedNumbers(ev)
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
