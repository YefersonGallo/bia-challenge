package app_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
)

const header = "meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor,status\n"

func row(meter string, ts time.Time, kwh float64) string {
	return fmt.Sprintf("%s,%s,%.2f,220.5,180.2,0.93,OK\n", meter, ts.UTC().Format("2006-01-02 15:04:05"), kwh)
}

func TestImportReadingsDryRunThenApply(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	last := dataset.Generate().Readings[len(dataset.Generate().Readings)-1]
	next := last.Timestamp.Add(time.Hour)
	csv := header + row(last.MeterID, last.Timestamp, 999) + row(last.MeterID, next, 50) + row(last.MeterID, next, 60)

	dry, err := s.ImportReadings(ctx, strings.NewReader(csv), true)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Applied || dry.Rows != 3 || dry.Added != 1 || dry.Replaced != 1 || dry.Duplicates != 1 || len(dry.Warnings) != 1 {
		t.Fatalf("dry run = %+v", dry)
	}
	before, _ := s.Meter(ctx, last.MeterID)

	res, err := s.ImportReadings(ctx, strings.NewReader(csv), false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Added != 1 || res.Replaced != 1 {
		t.Fatalf("import = %+v", res)
	}
	// The cached statistics are rebuilt: the meter now ends one hour later.
	after, _ := s.Meter(ctx, last.MeterID)
	if after.Stats.Readings != before.Stats.Readings+1 {
		t.Fatalf("readings %d → %d, want +1", before.Stats.Readings, after.Stats.Readings)
	}
	// Importing the same file again only replaces.
	again, _ := s.ImportReadings(ctx, strings.NewReader(csv), true)
	if again.Added != 0 || again.Replaced != 2 {
		t.Fatalf("second dry run = %+v", again)
	}
}

func TestImportReadingsRejectsBadFiles(t *testing.T) {
	s := newService(t)
	ts := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct{ csv, want string }{
		"unknown meter":  {header + row("M-999", ts, 1), "M-999 (1 filas)"},
		"missing column": {"meter_id,timestamp,consumption_kwh\nM-101,2026-09-20 00:00:00,1\n", `falta la columna "voltage_v"`},
		"empty value":    {header + "M-101,2026-09-20 00:00:00,,220,180,0.9,OK\n", "línea 2: consumption_kwh: valor vacío"},
		"bad date":       {header + "M-101,ayer,1,220,180,0.9,OK\n", "línea 2: fecha inválida"},
		"NaN":            {header + "M-101,2026-09-20 00:00:00,NaN,220,180,0.9,OK\n", "número inválido"},
		"no rows":        {header, "no tiene lecturas"},
		"wrong format":   {"meter_id,anomaly_type,severity\nM-109,REAL_ANOMALY,HIGH\n", "falta la columna"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := s.ImportReadings(context.Background(), strings.NewReader(c.csv), false)
			var ie *app.ImportError
			if !errors.As(err, &ie) || !errors.Is(err, app.ErrInvalid) || !strings.Contains(ie.Msg, c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}
