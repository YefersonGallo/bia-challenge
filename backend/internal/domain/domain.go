// Package domain holds the core entities of the energy management platform.
// It has no dependencies on infrastructure: storage, HTTP and AI adapters
// depend on it, never the other way around.
package domain

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
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
	EventDataQuality  EventKind = "DATA_QUALITY"  // a known metering / telemetry problem
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

// EventCategory says whether an event can explain a change in consumption.
type EventCategory string

const (
	EventExplanatory    EventCategory = "EXPLANATORY"     // a real operational cause (new line, outage)
	EventNonExplanatory EventCategory = "NON_EXPLANATORY" // "no operational event reported"
	EventInformational  EventCategory = "INFORMATIONAL"   // context only (e.g. a data-quality note)
)

// EventEffect is the direction of the change an event is expected to cause.
type EventEffect string

const (
	EffectUp   EventEffect = "UP"
	EffectDown EventEffect = "DOWN"
	EffectNone EventEffect = "NONE"
)

// Category classifies the event for correlation.
func (e Event) Category() EventCategory {
	switch e.Kind() {
	case EventLoadIncrease, EventLoadDecrease, EventShutdown:
		return EventExplanatory
	case EventDataQuality:
		return EventInformational
	default:
		return EventNonExplanatory
	}
}

// ExpectedEffect is the direction of consumption change the event implies.
func (e Event) ExpectedEffect() EventEffect {
	switch e.Kind() {
	case EventLoadIncrease:
		return EffectUp
	case EventLoadDecrease, EventShutdown:
		return EffectDown
	default:
		return EffectNone
	}
}

var durationRe = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(hours?|hrs?|h|horas?|days?|d[ií]as?)\b`)

// DurationHours extracts a duration from the description ("for 12 hours",
// "(72 h)", "3 días"); 0 when the event does not state one.
func (e Event) DurationHours() float64 {
	m := durationRe.FindStringSubmatch(e.Description)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
	if err != nil {
		return 0
	}
	if u := strings.ToLower(m[2]); strings.HasPrefix(u, "d") {
		v *= 24
	}
	return v
}

// MarshalJSON adds the derived attributes so clients and evidence carry them.
func (e Event) MarshalJSON() ([]byte, error) {
	type plain Event
	return json.Marshal(struct {
		plain
		Category       EventCategory `json:"category"`
		ExpectedEffect EventEffect   `json:"expected_effect"`
		DurationHours  float64       `json:"duration_hours,omitempty"`
	}{plain(e), e.Category(), e.ExpectedEffect(), e.DurationHours()})
}

// Kind maps the free-text event type from events.csv to a load effect.
func (e Event) Kind() EventKind {
	switch strings.ToUpper(e.Type) {
	case "UNKNOWN", "NONE", "N/A":
		// An explicit "no known cause" record never explains a change.
		return EventOther
	case "DATA_QUALITY", "METER_FAULT", "COMMUNICATION_ERROR":
		return EventDataQuality
	case "PRODUCTION_LINE_START", "NEW_PRODUCTION_LINE", "LOAD_INCREASE", "SHIFT_ADDED":
		return EventLoadIncrease
	case "SHIFT_REMOVED", "LOAD_DECREASE":
		return EventLoadDecrease
	case "SCHEDULED_SHUTDOWN", "PLANNED_SHUTDOWN", "SCHEDULED_OUTAGE", "PLANNED_OUTAGE", "MAINTENANCE", "SHUTDOWN":
		return EventShutdown
	}
	// Other codes (real datasets name events freely) are read word by word:
	// first the type; only a generic "change" type falls back to the words of its
	// description, and never when the description is negated ("no new equipment").
	// A type with no known word (NOTE, COMMENT…) never explains a change.
	if k := kindOfWords(words(e.Type)); k != EventOther {
		return k
	}
	typeWords := words(e.Type)
	if !typeWords["CHANGE"] && !typeWords["CAMBIO"] && !typeWords["OPERATIONAL"] && !typeWords["OPERACIONAL"] && !typeWords["OPERATION"] && !typeWords["OPERACION"] {
		return EventOther
	}
	desc := words(e.Description)
	for _, neg := range []string{"NO", "NOT", "NEVER", "WITHOUT", "SIN", "NUNCA", "NINGUN", "NINGUNA", "NI"} {
		if desc[neg] {
			return EventOther
		}
	}
	return kindOfWords(desc)
}

// words splits free text into upper-case words without accents, so "LINE" does
// not match inside "OFFLINE" and "línea" matches "LINEA".
func words(s string) map[string]bool {
	r := strings.NewReplacer("Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N")
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(r.Replace(strings.ToUpper(s)), func(c rune) bool {
		return !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9')
	}) {
		out[w] = true
	}
	return out
}

var kindWords = []struct {
	kind  EventKind
	words []string
}{
	// Shutdowns first: "parada de la línea" mentions a line too.
	{EventDataQuality, []string{"QUALITY", "CALIDAD", "TELEMETRY", "TELEMETRIA", "INTERMITTENT", "INTERMITENTE", "COMMUNICATION", "COMUNICACION", "FAULT"}},
	{EventShutdown, []string{"SHUTDOWN", "OUTAGE", "PARADA", "APAGADO", "MANTENIMIENTO", "MAINTENANCE", "STOP", "STOPPED", "CORTE"}},
	{EventLoadDecrease, []string{"DECREASE", "DECLINE", "DECLINED", "DROP", "DROPPED", "LOWER", "LOWERED", "REDUCE", "REDUCTION", "REDUCCION", "REDUCED", "REMOVED", "RETIRO", "RETIRADA", "DISMINUCION", "CAIDA", "DESCENSO", "BAJA"}},
	{EventLoadIncrease, []string{"START", "STARTUP", "STARTED", "ARRANQUE", "NEW", "NUEVA", "NUEVO", "INCREASE", "AUMENTO", "EXPANSION", "ADDED", "ACTIVATED", "ACTIVADA", "ACTIVADO", "INSTALLED", "INSTALADA", "INSTALADO", "LINE", "LINEA"}},
}

func kindOfWords(ws map[string]bool) EventKind {
	for _, k := range kindWords {
		for _, w := range k.words {
			if ws[w] {
				return k.kind
			}
		}
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
	MeterID           string           `json:"meter_id"`
	MeterName         string           `json:"meter_name"`
	BaselineKWh       float64          `json:"baseline_kwh"`
	CurrentKWh        float64          `json:"current_kwh"`
	VariationPct      float64          `json:"variation_pct"`
	OnsetDay          int              `json:"onset_day,omitempty"`
	EndDay            int              `json:"end_day,omitempty"`
	PersistentHours   int              `json:"persistent_hours"`
	NightRatio        float64          `json:"night_ratio"`
	InvalidReadings   int              `json:"invalid_readings"`
	PhysicalCoherence float64          `json:"physical_coherence"`
	Signals           []Signal         `json:"signals"`
	Variables         []VariableChange `json:"variables"`
	RelatedEvents     []Event          `json:"related_events"`
	// ShiftPct is the mean deviation of the episode (the change an event may
	// explain); VariationPct is the current day against the baseline.
	ShiftPct           float64    `json:"shift_pct"`
	Onset              *time.Time `json:"onset,omitempty"`
	EventExplainsShift bool       `json:"event_explains_shift"`
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

	// EvidenceSummary is a one-line summary of the evidence, written with the explanation.
	EvidenceSummary string `json:"evidence_summary"`
	// ConfidenceBreakdown is how the confidence was computed (weighted components).
	ConfidenceBreakdown []ConfidenceComponent `json:"confidence_breakdown"`
	// Impact is the projected cost of the anomaly (energy, money, reactive power).
	Impact *Impact `json:"projected_impact,omitempty"`
	// ChangePointAt / EndedAt bound the change (EndedAt is nil while it persists).
	ChangePointAt *time.Time `json:"change_point_at,omitempty"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
}

// ConfidenceComponent is one weighted term of the confidence.
type ConfidenceComponent struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
	Score  float64 `json:"score"` // 0..1
	Detail string  `json:"detail"`
}

// Impact quantifies what an anomaly costs.
type Impact struct {
	ExtraKWhPerDay   float64 `json:"extra_kwh_per_day"`
	ExtraKWhPerMonth float64 `json:"extra_kwh_per_month"`
	ExtraKWhSoFar    float64 `json:"extra_kwh_so_far"`
	CostPerMonthCOP  float64 `json:"cost_per_month_cop"`
	TariffCOPPerKWh  float64 `json:"tariff_cop_per_kwh"`
	PowerFactor      float64 `json:"power_factor"`
	ReactiveRatio    float64 `json:"reactive_ratio"`  // kVArh / kWh
	ReactiveExcess   float64 `json:"reactive_excess"` // share above 0.5, 0 if none
	ReactiveKVArhDay float64 `json:"reactive_kvarh_day"`
	Normalized       float64 `json:"normalized"` // 0..1, used in the priority score
}

// AnomalyAction is an operator action recorded on an anomaly.
type AnomalyAction struct {
	ID        string        `json:"id"`
	AnomalyID string        `json:"anomaly_id"`
	Action    string        `json:"action"`
	Note      string        `json:"note"`
	Status    AnomalyStatus `json:"status,omitempty"` // status after the action, when it changed it
	Actor     string        `json:"actor"`
	At        time.Time     `json:"at"`
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
	Key        string    `json:"key"`
	Label      string    `json:"label"`
	Status     RunStatus `json:"status"`
	Result     string    `json:"result,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
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
