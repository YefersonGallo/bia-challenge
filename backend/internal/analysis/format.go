package analysis

import (
	"fmt"
	"math"
	"strings"
)

// Spanish number formatting (decimal comma, dot thousands) used in evidence
// texts. The explainer validator relies on the same format.

func fmtNum(v float64, dec int) string {
	s := fmt.Sprintf("%.*f", dec, v)
	intPart, frac, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	out := b.String()
	if neg {
		out = "−" + out
	}
	if frac != "" {
		out += "," + frac
	}
	return out
}

func fmtInt(v int) string { return fmtNum(float64(v), 0) }

// fmtPct formats a signed percentage: +103,7%
func fmtPct(v float64) string {
	sign := ""
	if v > 0.05 {
		sign = "+"
	}
	if math.Abs(v) < 0.05 {
		v = 0
	}
	return sign + fmtNum(v, 1) + "%"
}

func fmtPctPlain(v float64) string { return fmtNum(v, 0) + "%" }

// FormatNumber is exported for other packages (explainer templates).
func FormatNumber(v float64, dec int) string { return fmtNum(v, dec) }

// FormatPct is exported for other packages.
func FormatPct(v float64) string { return fmtPct(v) }
