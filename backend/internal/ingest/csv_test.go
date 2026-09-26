package ingest

import (
	"strings"
	"testing"

	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
)

func TestReadReadingsToleratesColumnOrderAndCommaDecimals(t *testing.T) {
	in := "timestamp,meter_id,power_factor,current_a,voltage_v,consumption_kwh,status\n" +
		"2026-09-01 00:00:00,M-101,\"0,92\",23.1,220.4,4.66,OK\n"
	rs, err := ReadReadings(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].MeterID != "M-101" || rs[0].PowerFactor != 0.92 || rs[0].ConsumptionKWh != 4.66 {
		t.Fatalf("unexpected reading: %+v", rs)
	}
}

func TestReadReadingsRejectsMissingColumns(t *testing.T) {
	if _, err := ReadReadings(strings.NewReader("meter_id,timestamp\nM-1,2026-09-01T00:00:00Z\n")); err == nil {
		t.Fatal("expected an error for a missing consumption column")
	}
}

func TestReadReadingsReportsBadLine(t *testing.T) {
	in := "meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor\nM-1,not-a-date,1,220,5,0.9\n"
	_, err := ReadReadings(strings.NewReader(in))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected a line-numbered error, got %v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	ds := dataset.Generate()
	dir := t.TempDir()
	if err := WriteDir(dir, Data{Meters: ds.Meters, Readings: ds.Readings, Events: ds.Events}); err != nil {
		t.Fatal(err)
	}
	d, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Readings) != 4032 || len(d.Meters) != 12 || len(d.Events) != 2 {
		t.Fatalf("loaded %d readings, %d meters, %d events", len(d.Readings), len(d.Meters), len(d.Events))
	}
	if d.Meters[8].Name != "Compresor línea 3" {
		t.Errorf("meter names not loaded: %+v", d.Meters[8])
	}
}

// The official events.csv uses event_timestamp / event_type and has no id column.
func TestReadEventsOfficialFormat(t *testing.T) {
	in := "meter_id,event_timestamp,event_type,description\n" +
		"M-104,2026-09-11 00:00,OPERATIONAL_CHANGE,New production line activated\n" +
		"M-109,2026-09-12 14:00,UNKNOWN,No operational event reported\n"
	evs, err := ReadEvents(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].ID != "EV-001" || evs[1].Type != "UNKNOWN" || evs[1].Timestamp.Hour() != 14 {
		t.Fatalf("unexpected events: %+v", evs)
	}
}

func TestReadReadingsRequiresEveryValue(t *testing.T) {
	for name, in := range map[string]string{
		"missing voltage column": "meter_id,timestamp,consumption_kwh,current_a,power_factor\nM-1,2026-09-01 00:00,1,5,0.9\n",
		"empty power factor":     "meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor\nM-1,2026-09-01 00:00,1,220,5,\n",
		"empty meter":            "meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor\n,2026-09-01 00:00,1,220,5,0.9\n",
	} {
		if _, err := ReadReadings(strings.NewReader(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	_, err := ReadReadings(strings.NewReader("meter_id,timestamp,consumption_kwh,voltage_v,current_a,power_factor\nM-1,2026-09-01 00:00,1,220,5,0.9\nM-1,2026-09-01 01:00,1,,5,0.9\n"))
	if err == nil || !strings.Contains(err.Error(), "line 3") || !strings.Contains(err.Error(), "voltage_v") {
		t.Fatalf("empty cell: %v", err)
	}
}

func TestReadsSemicolonSeparatedFiles(t *testing.T) {
	in := "meter_id;timestamp;consumption_kwh;voltage_v;current_a;power_factor\nM-1;2026-09-01 00:00;4,5;220,1;23;0,92\n"
	rs, err := ReadReadings(strings.NewReader(in))
	if err != nil || len(rs) != 1 || rs[0].ConsumptionKWh != 4.5 || rs[0].PowerFactor != 0.92 {
		t.Fatalf("rs = %+v, err = %v", rs, err)
	}
}

func TestReadEventsReportsTheLine(t *testing.T) {
	in := "meter_id,event_timestamp,event_type,description\nM-104,2026-09-11 00:00,OPERATIONAL_CHANGE,ok\nM-109,ayer,UNKNOWN,x\n"
	if _, err := ReadEvents(strings.NewReader(in)); err == nil || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("err = %v", err)
	}
}
