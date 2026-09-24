// Package dataset generates the deterministic synthetic dataset used for
// development and tests. It mirrors the scenarios described in the challenge:
// 12 meters, 14 days of hourly readings (4,032 rows) and two operational events.
//
// When the official readings.csv / events.csv are available they replace the
// generated files; nothing else in the system depends on this package.
package dataset

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Start is the first hour of the period.
var Start = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

const (
	Days  = 14
	Hours = Days * 24
)

type meterSpec struct {
	id, name, location string
	dailyKWh           float64 // baseline consumption per day
	driftPct           float64 // mild change applied to days 8-14
	pf                 float64
}

var specs = []meterSpec{
	{"M-101", "Compresor principal", "Planta Norte · L1", 114.3, 2.5, 0.92},
	{"M-102", "Iluminación bodega", "Bodega A", 57.9, 1.1, 0.95},
	{"M-103", "HVAC oficinas", "Edificio administrativo", 183.0, -3.2, 0.90},
	{"M-104", "Línea de ensamble 2", "Planta Norte · L2", 180.0, 0, 0.92},
	{"M-105", "Cuarto frío", "Planta Sur", 210.2, 4.0, 0.89},
	{"M-106", "Horno industrial", "Planta Sur", 126.3, 0, 0.92},
	{"M-107", "Bombas de agua", "Cuarto de bombas", 138.2, -0.8, 0.88},
	{"M-108", "Tablero general", "Subestación A", 282.7, 3.6, 0.93},
	{"M-109", "Compresor línea 3", "Planta Norte · L3", 152.9, 0, 0.93},
	{"M-110", "Data center", "Edificio administrativo", 186.9, 0.9, 0.96},
	{"M-111", "Taller de mantenimiento", "Planta Sur", 55.5, -2.1, 0.90},
	{"M-112", "Subestación B", "Subestación B", 100.0, -1.4, 0.92},
}

// Hour-of-day shape for an industrial load (kWh per hour for a 152.9 kWh day).
var baseShape = [24]float64{3.5, 3.5, 3.5, 3.5, 3.5, 3.5, 5, 6.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 4.6, 4.6, 4.6, 4.6, 3.5, 3.5}

// M-109 after the change: the machine no longer idles at night.
var m109Shape = [24]float64{9.1, 9.1, 9.1, 9.1, 9.1, 9.1, 11, 12.5, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 13.7, 13.7, 13.7, 13.7, 9.1, 9.1}

func shapeSum(s [24]float64) float64 {
	t := 0.0
	for _, v := range s {
		t += v
	}
	return t
}

// Dataset is the generated content.
type Dataset struct {
	Meters   []domain.Meter
	Readings []domain.Reading
	Events   []domain.Event
}

// Generate builds the dataset deterministically.
func Generate() Dataset {
	rng := rand.New(rand.NewSource(42))
	ds := Dataset{}
	baseSum := shapeSum(baseShape)
	for _, sp := range specs {
		ds.Meters = append(ds.Meters, domain.Meter{ID: sp.id, Name: sp.name, Location: sp.location, CreatedAt: Start})
		for h := 0; h < Hours; h++ {
			ts := Start.Add(time.Duration(h) * time.Hour)
			day := h/24 + 1
			hod := h % 24
			kwh := sp.dailyKWh * baseShape[hod] / baseSum
			if day >= 8 {
				kwh *= 1 + sp.driftPct/100
			}
			voltage := 220 + rng.NormFloat64()*0.8
			pf := sp.pf + rng.NormFloat64()*0.004
			status := "OK"

			switch sp.id {
			case "M-104": // new production line from day 8, 06:00
				if h >= 7*24+6 {
					kwh *= 1.476
				}
			case "M-106": // scheduled shutdown days 10-12
				if day >= 10 && day <= 12 {
					kwh *= 0.14
				}
			case "M-109": // real anomaly: doubles from day 8, worse power factor
				if day >= 8 {
					kwh = sp.dailyKWh * 2.037 * m109Shape[hod] / shapeSum(m109Shape)
					pf = 0.81 + rng.NormFloat64()*0.004
					voltage = 216 + rng.NormFloat64()*0.8
				}
			}
			kwh *= 1 + rng.NormFloat64()*0.02
			kwh = math.Max(kwh, 0.05)
			current := kwh * 1000 / (voltage * pf)

			if sp.id == "M-112" {
				// data quality problem: readings that are physically impossible
				if hod == 14 && dayIn(day, 3, 5, 7, 9, 11, 13) {
					voltage = 0 // six hours with 0 V but positive consumption
					status = "SUSPECT"
				}
				if isPFGlitch(h) {
					pf = 1.03 + float64(h%5)*0.01
					status = "SUSPECT"
				}
				if h%8 == 3 { // current transformer drift: kWh no longer matches V·I·PF
					current *= 1.45
				}
			}
			ds.Readings = append(ds.Readings, domain.Reading{
				MeterID: sp.id, Timestamp: ts,
				ConsumptionKWh: round(kwh, 3), VoltageV: round(voltage, 1), CurrentA: round(current, 2), PowerFactor: round(pf, 3), Status: status,
			})
		}
	}
	ds.Events = []domain.Event{
		{ID: "EV-001", MeterID: "M-104", Timestamp: Start.Add((7*24 + 6) * time.Hour), Type: "PRODUCTION_LINE_START", Description: "Arranque de la nueva línea productiva 2B"},
		{ID: "EV-002", MeterID: "M-106", Timestamp: Start.Add(9 * 24 * time.Hour), Type: "SCHEDULED_SHUTDOWN", Description: "Parada programada de mantenimiento del horno (72 h)"},
	}
	return ds
}

// pfGlitchHours are the 19 hours in which M-112 reports a power factor above 1.
var pfGlitchHours = func() map[int]bool {
	m := map[int]bool{}
	for i := 0; i < 19; i++ {
		m[2*24+10+i*15] = true
	}
	return m
}()

func isPFGlitch(h int) bool { return pfGlitchHours[h] }

func dayIn(day int, days ...int) bool {
	for _, d := range days {
		if d == day {
			return true
		}
	}
	return false
}

func round(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}

// MeterIDs returns the ids in generation order.
func MeterIDs() []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.id
	}
	return out
}

// Describe returns a one-line description, handy for logs.
func (d Dataset) Describe() string {
	return fmt.Sprintf("%d meters · %d readings · %d events", len(d.Meters), len(d.Readings), len(d.Events))
}
