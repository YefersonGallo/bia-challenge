package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
	"github.com/yefersongallo/bia-energy/backend/internal/ingest"
)

// ImportResult summarises a readings CSV: what it contains and what importing it
// changes (or changed, when Applied).
type ImportResult struct {
	Rows       int       `json:"rows"`       // readings in the file
	Added      int       `json:"added"`      // new meter-hour pairs
	Replaced   int       `json:"replaced"`   // meter-hour pairs already stored, overwritten
	Duplicates int       `json:"duplicates"` // repeated meter-hour pairs inside the file (the last one wins)
	Meters     []string  `json:"meters"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Warnings   []string  `json:"warnings"`
	Applied    bool      `json:"applied"`
}

// ImportError is a CSV the platform cannot take; its message is shown to the user.
type ImportError struct{ Msg string }

func (e *ImportError) Error() string { return e.Msg }
func (e *ImportError) Unwrap() error { return ErrInvalid }

// ImportReadings reads a CSV in the format of readings.csv and, unless dryRun,
// upserts it: same meter and hour replaces, anything else is added. The file is
// rejected as a whole on the first malformed line or on meters that do not exist.
// The status column, when present, is stored but never used by the analysis.
func (s *Service) ImportReadings(ctx context.Context, r io.Reader, dryRun bool) (ImportResult, error) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	rs, err := ingest.ReadReadings(r)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return ImportResult{}, &ImportError{Msg: "el archivo supera el tamaño máximo"}
		}
		return ImportResult{}, &ImportError{Msg: spanishCSVError(err)}
	}
	if len(rs) == 0 {
		return ImportResult{}, &ImportError{Msg: "el archivo no tiene lecturas"}
	}

	meters, err := s.store.Meters(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	known := make(map[string]bool, len(meters))
	for _, m := range meters {
		known[m.ID] = true
	}

	type key struct {
		meter string
		ts    int64
	}
	last := map[key]int{}
	unknown := map[string]int{}
	offHour := 0
	res := ImportResult{Rows: len(rs), From: rs[0].Timestamp, To: rs[0].Timestamp, Warnings: []string{}}
	seen := map[string]bool{}
	for i, x := range rs {
		if !known[x.MeterID] {
			unknown[x.MeterID]++
			continue
		}
		k := key{x.MeterID, x.Timestamp.Unix()}
		if _, dup := last[k]; dup {
			res.Duplicates++
		}
		last[k] = i
		if !seen[x.MeterID] {
			seen[x.MeterID] = true
			res.Meters = append(res.Meters, x.MeterID)
		}
		if x.Timestamp.Before(res.From) {
			res.From = x.Timestamp
		}
		if x.Timestamp.After(res.To) {
			res.To = x.Timestamp
		}
		if x.Timestamp.Minute() != 0 || x.Timestamp.Second() != 0 {
			offHour++
		}
	}
	if len(unknown) > 0 {
		ids := make([]string, 0, len(unknown))
		for id, n := range unknown {
			ids = append(ids, fmt.Sprintf("%s (%d filas)", id, n))
		}
		sort.Strings(ids)
		return ImportResult{}, &ImportError{Msg: "medidores que no existen en la planta: " + strings.Join(ids, ", ")}
	}
	sort.Strings(res.Meters)
	if offHour > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d lecturas no caen en la hora en punto: el análisis las agrupa por hora", offHour))
	}
	if res.Duplicates > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d lecturas repetidas para el mismo medidor y hora: se toma la última", res.Duplicates))
	}

	// The last reading of each meter-hour, in file order.
	idx := make([]int, 0, len(last))
	for _, i := range last {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	batch := make([]domain.Reading, len(idx))
	for j, i := range idx {
		batch[j] = rs[i]
	}

	existing, err := s.store.Readings(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	stored := make(map[key]bool, len(existing))
	for _, x := range existing {
		stored[key{x.MeterID, x.Timestamp.Unix()}] = true
	}
	for _, x := range batch {
		if stored[key{x.MeterID, x.Timestamp.Unix()}] {
			res.Replaced++
		} else {
			res.Added++
		}
	}
	if dryRun {
		return res, nil
	}

	added, replaced, err := s.store.UpsertReadings(ctx, batch)
	if err != nil {
		return ImportResult{}, err
	}
	res.Added, res.Replaced, res.Applied = added, replaced, true
	s.statsMu.Lock()
	s.stats = nil // the rule-layer statistics are recomputed from the new readings
	s.statsMu.Unlock()
	s.opts.Logger.Info("readings imported", "rows", res.Rows, "added", added, "replaced", replaced)
	if s.opts.OnReadingsChanged != nil {
		s.opts.OnReadingsChanged()
	}
	return res, nil
}

var csvErrorWords = strings.NewReplacer(
	"line ", "línea ",
	"missing column", "falta la columna",
	"empty value", "valor vacío",
	"invalid timestamp", "fecha inválida",
	"invalid number", "número inválido",
	"strconv.ParseFloat: parsing", "número inválido:",
	": invalid syntax", "",
	"wrong number of fields", "número de columnas incorrecto",
	"empty file", "archivo vacío",
)

func spanishCSVError(err error) string { return csvErrorWords.Replace(err.Error()) }
