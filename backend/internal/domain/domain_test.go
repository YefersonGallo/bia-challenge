package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAnomalyLifecycleTransitions(t *testing.T) {
	cases := []struct {
		from, to AnomalyStatus
		ok       bool
	}{
		{AnomalyOpen, AnomalyAcknowledged, true},
		{AnomalyOpen, AnomalyResolved, true},
		{AnomalyOpen, AnomalyInProgress, false},
		{AnomalyAcknowledged, AnomalyInProgress, true},
		{AnomalyInProgress, AnomalyResolved, true},
		{AnomalyResolved, AnomalyOpen, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.ok {
			t.Errorf("%s → %s = %v, want %v", c.from, c.to, got, c.ok)
		}
	}
}

func TestEventKind(t *testing.T) {
	cases := map[string]EventKind{
		"PRODUCTION_LINE_START": EventLoadIncrease,
		"SCHEDULED_SHUTDOWN":    EventShutdown,
		"MAINTENANCE":           EventShutdown,
		"SHIFT_REMOVED":         EventLoadDecrease,
		"SOMETHING_ELSE":        EventOther,
		"line_start":            EventLoadIncrease,
		"PARADA_PROGRAMADA":     EventShutdown,
	}
	for typ, want := range cases {
		if got := (Event{Type: typ, Timestamp: time.Now()}).Kind(); got != want {
			t.Errorf("%s → %s, want %s", typ, got, want)
		}
	}
}

func TestEventKindFromDescription(t *testing.T) {
	cases := []struct {
		desc string
		want EventKind
	}{
		{"Parada programada de la línea 2", EventShutdown},
		{"Arranque de nueva línea productiva", EventLoadIncrease},
		{"Reducción de turno nocturno", EventLoadDecrease},
		{"Visita de auditoría", EventOther},
	}
	for _, c := range cases {
		if got := (Event{Type: "OPERATIONAL", Description: c.desc}).Kind(); got != c.want {
			t.Errorf("%q → %s, want %s", c.desc, got, c.want)
		}
	}
}

func TestFalsePositiveIsNotAnAnomaly(t *testing.T) {
	if (Anomaly{Type: FalsePositive}).IsAnomaly() || !(Anomaly{Type: RealAnomaly}).IsAnomaly() {
		t.Fatal("IsAnomaly must be false only for false positives")
	}
}

func TestEventAttributes(t *testing.T) {
	cases := []struct {
		ev       Event
		cat      EventCategory
		eff      EventEffect
		duration float64
	}{
		{Event{Type: "OPERATIONAL_CHANGE", Description: "New production line activated"}, EventExplanatory, EffectUp, 0},
		{Event{Type: "SCHEDULED_OUTAGE", Description: "Scheduled maintenance outage for 12 hours"}, EventExplanatory, EffectDown, 12},
		{Event{Type: "UNKNOWN", Description: "No operational event reported"}, EventNonExplanatory, EffectNone, 0},
		{Event{Type: "DATA_QUALITY", Description: "Intermittent readings and abnormal electrical jumps"}, EventInformational, EffectNone, 0},
		{Event{Type: "SCHEDULED_SHUTDOWN", Description: "Parada programada del horno (72 h)"}, EventExplanatory, EffectDown, 72},
		{Event{Type: "MAINTENANCE", Description: "Mantenimiento de 3 días"}, EventExplanatory, EffectDown, 72},
	}
	for _, c := range cases {
		if c.ev.Category() != c.cat || c.ev.ExpectedEffect() != c.eff || c.ev.DurationHours() != c.duration {
			t.Errorf("%s %q → %s %s %.0f", c.ev.Type, c.ev.Description, c.ev.Category(), c.ev.ExpectedEffect(), c.ev.DurationHours())
		}
	}
	b, _ := json.Marshal(cases[1].ev)
	if !strings.Contains(string(b), `"duration_hours":12`) || !strings.Contains(string(b), `"category":"EXPLANATORY"`) {
		t.Errorf("json = %s", b)
	}
}

func TestEventKindReadsWordsNotSubstrings(t *testing.T) {
	cases := []struct {
		typ, desc string
		want      EventKind
	}{
		{"OPERATIONAL_CHANGE", "New production line activated", EventLoadIncrease},
		{"OPERATIONAL_CHANGE", "Meter went offline", EventOther},                     // "LINE" inside "OFFLINE"
		{"OPERATIONAL_CHANGE", "Demand decline after the season", EventLoadDecrease}, // "DECLINE", not "LINE"
		{"OPERATIONAL_CHANGE", "Production decline on line 2", EventLoadDecrease},    // a decrease wins over "line"
		{"NOTE", "No new equipment was installed on this line", EventOther},          // unknown type: text never decides
		{"OPERATIONAL_CHANGE", "No new equipment was installed", EventOther},         // negated description
		{"CAMBIO_OPERATIVO", "Arranque de la nueva línea", EventLoadIncrease},
		{"LINE_STOP", "", EventShutdown},
		{"UNKNOWN", "New production line activated", EventOther},
	}
	for _, c := range cases {
		if got := (Event{Type: c.typ, Description: c.desc}).Kind(); got != c.want {
			t.Errorf("%s %q = %v, want %v", c.typ, c.desc, got, c.want)
		}
	}
}
