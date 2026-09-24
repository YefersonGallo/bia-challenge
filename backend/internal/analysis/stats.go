package analysis

import (
	"math"
	"sort"
)

// median returns the median of xs (0 for an empty slice). xs is not modified.
func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

// mad is the median absolute deviation, a robust estimate of spread.
func mad(xs []float64, med float64) float64 {
	d := make([]float64, len(xs))
	for i, x := range xs {
		d[i] = math.Abs(x - med)
	}
	return median(d)
}

// robustZ scores x against a median/MAD baseline. The 1.4826 factor makes the
// MAD consistent with a standard deviation for normal data; a floor avoids
// dividing by ~0 when a baseline is perfectly flat.
func robustZ(x, med, madv float64) float64 {
	scale := 1.4826 * madv
	floor := math.Max(math.Abs(med)*0.02, 1e-6)
	if scale < floor {
		scale = floor
	}
	return (x - med) / scale
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

func pctChange(base, cur float64) float64 {
	if base == 0 {
		return 0
	}
	return (cur/base - 1) * 100
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func round(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}
