// Package domain holds the core entities of the energy management platform.
// It has no dependencies on infrastructure: storage, HTTP and AI adapters
// depend on it, never the other way around.
package domain

import (
	"errors"
	"strings"
	"time"
)

// Meter is an electrical meter installed in the plant.
type Meter struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Location  string    `json:"location"`
	CreatedAt time.Time `json:"created_at"`
}

// Reading is one hourly measurement of a meter.
type Reading struct {
	MeterID        string    `json:"meter_id"`
	Timestamp      time.Time `json:"timestamp"`
	ConsumptionKWh float64   `json:"consumption_kwh"`
	VoltageV       float64   `json:"voltage_v"`
	CurrentA       float64   `json:"current_a"`
	PowerFactor    float64   `json:"power_factor"`
	Status         string    `json:"status"`
}

// EventKind classifies known operational events by the effect they have on load.
type EventKind string

const (
	EventLoadIncrease EventKind = "LOAD_INCREASE" // e.g. a new production line starts
	EventLoadDecrease EventKind = "LOAD_DECREASE" // e.g. a shift is removed
	EventShutdown     EventKind = "SHUTDOWN"      // scheduled stop / maintenance
	EventOther        EventKind = "OTHER"
)

// Event is a known operational event that may explain a change in a meter.
type Event struct {
	ID          string    `json:"id"`
	MeterID     string    `json:"meter_id"`
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
}

// Kind maps the free-text event type from events.csv to a load effect.
func (e Event) Kind() EventKind {
	switch strings.ToUpper(e.Type) {
	case "PRODUCTION_LINE_START", "NEW_PRODUCTION_LINE", "LOAD_INCREASE", "SHIFT_ADDED":
		return EventLoadIncrease
	case "SHIFT_REMOVED", "LOAD_DECREASE":
		return EventLoadDecrease
	case "SCHEDULED_SHUTDOWN", "PLANNED_SHUTDOWN", "MAINTENANCE", "SHUTDOWN":
		return EventShutdown
	}
	// Unknown codes (real datasets name events freely): fall back to keywords in
	// the type and the description, in Spanish and English. Shutdowns are checked
	// first because "parada de la línea" mentions a line too.
	text := strings.ToUpper(e.Type + " " + e.Description)
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(text, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has("SHUTDOWN", "PARADA", "APAGADO", "MANTENIMIENTO", "MAINTENANCE", "STOP", "OUTAGE"):
		return EventShutdown
	case has("DECREASE", "REDUC", "REMOVED", "DISMINU", "RETIRO"):
		return EventLoadDecrease
	case has("START", "ARRANQUE", "NEW", "NUEVA", "NUEVO", "INCREASE", "AUMENTO", "EXPANSI", "ADDED", "LÍNEA", "LINEA", "LINE"):
		return EventLoadIncrease
	}
	return EventOther
}

// MeterStatus is the rule-based state of a meter, computed without AI.
type MeterStatus string

const (
	StatusOK       MeterStatus = "OK"
	StatusAlert    MeterStatus = "ALERT"
	StatusCritical MeterStatus = "CRITICAL"
)

// AnomalyType is the verdict of the analysis engine.
type AnomalyType string

const (
	RealAnomaly        AnomalyType = "REAL_ANOMALY"
	ExplainableAnomaly AnomalyType = "EXPLAINABLE_ANOMALY"
	FalsePositive      AnomalyType = "FALSE_POSITIVE"
	DataQuality        AnomalyType = "DATA_QUALITY"
)

// Severity of a finding.
type Severity string

const (
	SeverityHigh   Severity = "HIGH"
	SeverityMedium Severity = "MEDIUM"
	SeverityLow    Severity = "LOW"
)

// Weight turns a severity into a number used by the priority score.
func (s Severity) Weight() float64 {
	switch s {
	case SeverityHigh:
		return 1.0
	case SeverityMedium:
		return 0.6
	default:
		return 0.3
	}
}

// AnomalyStatus is the operational lifecycle of an anomaly.
type AnomalyStatus string

const (
	AnomalyOpen         AnomalyStatus = "OPEN"
	AnomalyAcknowledged AnomalyStatus = "ACKNOWLEDGED"
	AnomalyInProgress   AnomalyStatus = "IN_PROGRESS"
	AnomalyResolved     AnomalyStatus = "RESOLVED"
)

// ErrInvalidTransition is returned when an anomaly status change is not allowed.
var ErrInvalidTransition = errors.New("invalid anomaly status transition")

var transitions = map[AnomalyStatus][]AnomalyStatus{
	AnomalyOpen:         {AnomalyAcknowledged, AnomalyResolved},
	AnomalyAcknowledged: {AnomalyInProgress, AnomalyResolved},
	AnomalyInProgress:   {AnomalyResolved},
	AnomalyResolved:     {},
}

// CanTransition reports whether an anomaly may move from one status to another.
func CanTransition(from, to AnomalyStatus) bool {
	for _, s := range transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Signal is one piece of evidence produced by a detector.
type Signal struct {
	Code        string  `json:"code"`
	Description string  `json:"description"`
	Value       float64 `json:"value"`
}

// VariableChange compares a variable between the baseline and the analysis window.
type VariableChange struct {
	Name     string  `json:"name"`
	Unit     string  `json:"unit"`
	Baseline float64 `json:"baseline"`
	Current  float64 `json:"current"`
	DeltaPct float64 `json:"delta_pct"`
}

// Evidence is everything the engine computed for a finding. It is persisted
// with the anomaly and is the only input the explainer (LLM) receives.
type Evidence struct {
	MeterID            string           `json:"meter_id"`
	MeterName          string           `json:"meter_name"`
	BaselineKWh        float64          `json:"baseline_kwh"`
	CurrentKWh         float64          `json:"current_kwh"`
	VariationPct       float64          `json:"variation_pct"`
	OnsetDay           int              `json:"onset_day,omitempty"`
	EndDay             int              `json:"end_day,omitempty"`
	PersistentHours    int              `json:"persistent_hours"`
	NightRatio         float64          `json:"night_ratio"`
	InvalidReadings    int              `json:"invalid_readings"`
	PhysicalCoherence  float64          `json:"physical_coherence"`
	Signals            []Signal         `json:"signals"`
	Variables          []VariableChange `json:"variables"`
	RelatedEvents      []Event          `json:"related_events"`
	EventExplainsShift bool             `json:"event_explains_shift"`
}

// Anomaly is a classified finding with its explanation and recommended action.
type Anomaly struct {
	ID                string        `json:"id"`
	AnalysisID        string        `json:"analysis_id"`
	MeterID           string        `json:"meter_id"`
	DetectedAt        time.Time     `json:"detected_at"`
	Type              AnomalyType   `json:"type"`
	Severity          Severity      `json:"severity"`
	Confidence        float64       `json:"confidence"`
	PriorityScore     float64       `json:"priority_score"`
	Rank              int           `json:"rank"`
	Reason            string        `json:"reason"`
	RecommendedAction string        `json:"recommended_action"`
	NextSteps         []string      `json:"next_steps"`
	ExplainedBy       string        `json:"explained_by"`
	Status            AnomalyStatus `json:"status"`
	Evidence          Evidence      `json:"evidence"`
}

// IsAnomaly mirrors the `anomaly` boolean of the expected output format.
func (a Anomaly) IsAnomaly() bool { return a.Type != FalsePositive }

// RunStatus is the state of an analysis run.
type RunStatus string

const (
	RunPending   RunStatus = "PENDING"
	RunRunning   RunStatus = "RUNNING"
	RunCompleted RunStatus = "COMPLETED"
	RunFailed    RunStatus = "FAILED"
)

// StepState is the state of one pipeline step.
type StepState struct {
	Key    string    `json:"key"`
	Label  string    `json:"label"`
	Status RunStatus `json:"status"`
	Result string    `json:"result,omitempty"`
}

// RunSummary is shown when an analysis finishes.
type RunSummary struct {
	Anomalies     int     `json:"anomalies"`
	HighPriority  int     `json:"high_priority"`
	AvgConfidence float64 `json:"avg_confidence"`
	Headline      string  `json:"headline"`
}

// AnalysisRun tracks one execution of the AI analysis pipeline.
type AnalysisRun struct {
	ID          string      `json:"id"`
	Status      RunStatus   `json:"status"`
	StartedAt   time.Time   `json:"started_at"`
	FinishedAt  *time.Time  `json:"finished_at,omitempty"`
	CurrentStep int         `json:"current_step"`
	Steps       []StepState `json:"steps"`
	Summary     *RunSummary `json:"summary,omitempty"`
	Error       string      `json:"error,omitempty"`
}
