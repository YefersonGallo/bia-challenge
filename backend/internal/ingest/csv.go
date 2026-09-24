// Package ingest reads and writes the CSV files of the challenge:
// readings.csv, events.csv and (optional) meters.csv.
package ingest

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Data is the content of a data directory.
type Data struct {
	Meters   []domain.Meter
	Readings []domain.Reading
	Events   []domain.Event
}

// LoadDir reads readings.csv (required), events.csv and meters.csv (optional).
// expected_results.csv is deliberately never read: it is reserved for grading.
func LoadDir(dir string) (Data, error) {
	var d Data
	f, err := os.Open(filepath.Join(dir, "readings.csv"))
	if err != nil {
		return d, fmt.Errorf("open readings.csv: %w", err)
	}
	defer f.Close()
	if d.Readings, err = ReadReadings(f); err != nil {
		return d, err
	}
	if ef, err := os.Open(filepath.Join(dir, "events.csv")); err == nil {
		defer ef.Close()
		if d.Events, err = ReadEvents(ef); err != nil {
			return d, err
		}
	}
	names := map[string]domain.Meter{}
	if mf, err := os.Open(filepath.Join(dir, "meters.csv")); err == nil {
		defer mf.Close()
		ms, err := ReadMeters(mf)
		if err != nil {
			return d, err
		}
		for _, m := range ms {
			names[m.ID] = m
		}
	}
	d.Meters = metersFrom(d.Readings, names)
	return d, nil
}

func metersFrom(rs []domain.Reading, known map[string]domain.Meter) []domain.Meter {
	seen := map[string]bool{}
	var out []domain.Meter
	for _, r := range rs {
		if seen[r.MeterID] {
			continue
		}
		seen[r.MeterID] = true
		m, ok := known[r.MeterID]
		if !ok {
			m = domain.Meter{ID: r.MeterID, Name: r.MeterID, Location: "—"}
		}
		if m.CreatedAt.IsZero() {
			m.CreatedAt = r.Timestamp
		}
		out = append(out, m)
	}
	return out
}

// header maps column names (case-insensitive) to their index, so the loader
// tolerates column order changes and common aliases.
type header map[string]int

func readHeader(r *csv.Reader) (header, error) {
	cols, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	h := header{}
	for i, c := range cols {
		h[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(c, "\ufeff")))] = i
	}
	return h, nil
}

func (h header) get(rec []string, names ...string) string {
	for _, n := range names {
		if i, ok := h[n]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
	}
	return ""
}

func (h header) require(names ...string) error {
	for _, n := range names {
		if _, ok := h[n]; !ok {
			return fmt.Errorf("missing column %q", n)
		}
	}
	return nil
}

var timeLayouts = []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04"}

func parseTime(s string) (time.Time, error) {
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", s)
}

func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
}

// ReadReadings parses readings.csv.
func ReadReadings(r io.Reader) ([]domain.Reading, error) {
	cr := csv.NewReader(r)
	h, err := readHeader(cr)
	if err != nil {
		return nil, err
	}
	if err := h.require("meter_id", "timestamp", "consumption_kwh"); err != nil {
		return nil, err
	}
	var out []domain.Reading
	line := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		ts, err := parseTime(h.get(rec, "timestamp"))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		var vals [4]float64
		for i, col := range []string{"consumption_kwh", "voltage_v", "current_a", "power_factor"} {
			if vals[i], err = parseFloat(h.get(rec, col)); err != nil {
				return nil, fmt.Errorf("line %d: %s: %w", line, col, err)
			}
		}
		out = append(out, domain.Reading{
			MeterID: h.get(rec, "meter_id"), Timestamp: ts, ConsumptionKWh: vals[0], VoltageV: vals[1], CurrentA: vals[2], PowerFactor: vals[3],
			Status: h.get(rec, "status"),
		})
	}
	return out, nil
}

// ReadEvents parses events.csv.
func ReadEvents(r io.Reader) ([]domain.Event, error) {
	cr := csv.NewReader(r)
	h, err := readHeader(cr)
	if err != nil {
		return nil, err
	}
	if err := h.require("meter_id", "timestamp", "type"); err != nil {
		return nil, err
	}
	var out []domain.Event
	for i := 1; ; i++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		ts, err := parseTime(h.get(rec, "timestamp"))
		if err != nil {
			return nil, err
		}
		id := h.get(rec, "id")
		if id == "" {
			id = fmt.Sprintf("EV-%03d", i)
		}
		out = append(out, domain.Event{ID: id, MeterID: h.get(rec, "meter_id"), Timestamp: ts, Type: strings.ToUpper(h.get(rec, "type")), Description: h.get(rec, "description")})
	}
	return out, nil
}

// ReadMeters parses the optional meters.csv (meter_id,name,location).
func ReadMeters(r io.Reader) ([]domain.Meter, error) {
	cr := csv.NewReader(r)
	h, err := readHeader(cr)
	if err != nil {
		return nil, err
	}
	var out []domain.Meter
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Meter{ID: h.get(rec, "meter_id", "id"), Name: h.get(rec, "name"), Location: h.get(rec, "location")})
	}
	return out, nil
}

// WriteDir writes the three CSV files (used by cmd/gendata).
func WriteDir(dir string, d Data) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	write := func(name string, rows [][]string) error {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		defer f.Close()
		w := csv.NewWriter(f)
		if err := w.WriteAll(rows); err != nil {
			return err
		}
		return w.Error()
	}
	ff := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	rows := [][]string{{"meter_id", "timestamp", "consumption_kwh", "voltage_v", "current_a", "power_factor", "status"}}
	for _, r := range d.Readings {
		rows = append(rows, []string{r.MeterID, r.Timestamp.Format(time.RFC3339), ff(r.ConsumptionKWh), ff(r.VoltageV), ff(r.CurrentA), ff(r.PowerFactor), r.Status})
	}
	if err := write("readings.csv", rows); err != nil {
		return err
	}
	ev := [][]string{{"id", "meter_id", "timestamp", "type", "description"}}
	for _, e := range d.Events {
		ev = append(ev, []string{e.ID, e.MeterID, e.Timestamp.Format(time.RFC3339), e.Type, e.Description})
	}
	if err := write("events.csv", ev); err != nil {
		return err
	}
	ms := [][]string{{"meter_id", "name", "location"}}
	for _, m := range d.Meters {
		ms = append(ms, []string{m.ID, m.Name, m.Location})
	}
	return write("meters.csv", ms)
}
