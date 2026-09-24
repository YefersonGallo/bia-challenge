package domain

import (
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
