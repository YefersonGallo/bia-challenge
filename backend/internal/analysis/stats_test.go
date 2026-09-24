package analysis

import (
	"math"
	"testing"
)

func TestMedianAndMAD(t *testing.T) {
	cases := []struct {
		in       []float64
		med, mad float64
	}{
		{[]float64{}, 0, 0},
		{[]float64{3}, 3, 0},
		{[]float64{1, 2, 3, 4}, 2.5, 1},
		{[]float64{1, 1, 2, 2, 4, 6, 9}, 2, 1},
	}
	for _, c := range cases {
		m := median(c.in)
		if m != c.med {
			t.Errorf("median(%v) = %v, want %v", c.in, m, c.med)
		}
		if d := mad(c.in, m); d != c.mad {
			t.Errorf("mad(%v) = %v, want %v", c.in, d, c.mad)
		}
	}
}

func TestRobustZUsesFloorForFlatBaselines(t *testing.T) {
	z := robustZ(11, 10, 0)
	if math.IsInf(z, 0) || math.IsNaN(z) {
		t.Fatal("z must be finite with MAD = 0")
	}
	if z <= 0 {
		t.Errorf("z = %v, want positive", z)
	}
}

func TestSpanishFormatting(t *testing.T) {
	cases := map[string]string{
		fmtNum(2180, 0):      "2.180",
		fmtNum(0.81, 2):      "0,81",
		fmtPct(103.66):       "+103,7%",
		fmtPct(-38.94):       "−38,9%",
		fmtPct(0.01):         "0,0%",
		fmtNum(1234567.8, 1): "1.234.567,8",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
