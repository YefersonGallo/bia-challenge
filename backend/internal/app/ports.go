// Package app contains the use cases of the platform. It depends on the
// domain, the analysis engine and the explainer through interfaces (ports);
// storage and HTTP are adapters that live outside.
package app

import (
	"context"
	"errors"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// ErrNotFound is returned by repositories when an entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrInvalid means the request itself is not valid (bad parameters).
var ErrInvalid = errors.New("invalid request")

// ErrConflict signals an operation that is not valid in the current state.
var ErrConflict = errors.New("conflict")

// MeterRepository gives access to meters and their readings.
type MeterRepository interface {
	Meters(ctx context.Context) ([]domain.Meter, error)
	Readings(ctx context.Context) ([]domain.Reading, error)
	ReadingsByMeter(ctx context.Context, meterID string) ([]domain.Reading, error)
}

// EventRepository gives access to operational events.
type EventRepository interface {
	Events(ctx context.Context) ([]domain.Event, error)
}

// RunRepository persists analysis runs.
type RunRepository interface {
	SaveRun(ctx context.Context, run domain.AnalysisRun) error
	Run(ctx context.Context, id string) (domain.AnalysisRun, error)
	LatestRun(ctx context.Context) (domain.AnalysisRun, error)
}

// AnomalyRepository persists the findings of the latest completed run.
type AnomalyRepository interface {
	ReplaceAnomalies(ctx context.Context, runID string, as []domain.Anomaly) error
	Anomalies(ctx context.Context) ([]domain.Anomaly, error)
	Anomaly(ctx context.Context, id string) (domain.Anomaly, error)
	UpdateAnomalyStatus(ctx context.Context, id string, status domain.AnomalyStatus) error
}

// ActionRepository keeps the operator actions recorded on anomalies.
type ActionRepository interface {
	AddAction(ctx context.Context, a domain.AnomalyAction) error
	Actions(ctx context.Context, anomalyID string) ([]domain.AnomalyAction, error)
}

// ReadingWriter adds readings after the initial load (CSV import).
type ReadingWriter interface {
	// UpsertReadings inserts new readings and replaces the ones with the same
	// meter and timestamp. It returns how many were added and replaced.
	UpsertReadings(ctx context.Context, rs []domain.Reading) (added, replaced int, err error)
}

// Seeder loads the initial dataset.
type Seeder interface {
	Empty(ctx context.Context) (bool, error)
	Seed(ctx context.Context, meters []domain.Meter, readings []domain.Reading, events []domain.Event) error
}

// Store groups every repository an adapter must implement.
type Store interface {
	MeterRepository
	EventRepository
	RunRepository
	AnomalyRepository
	ActionRepository
	ReadingWriter
	Seeder
}
