// Package memory is an in-memory implementation of app.Store. It is used by
// tests and when the API runs without DATABASE_URL.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Store keeps everything in maps guarded by a RWMutex.
type Store struct {
	mu        sync.RWMutex
	meters    []domain.Meter
	readings  []domain.Reading
	events    []domain.Event
	runs      map[string]domain.AnalysisRun
	anomalies []domain.Anomaly // of the latest completed run
}

var _ app.Store = (*Store)(nil)

// New returns an empty store.
func New() *Store { return &Store{runs: map[string]domain.AnalysisRun{}} }

func (s *Store) Empty(context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.readings) == 0, nil
}

func (s *Store) Seed(_ context.Context, ms []domain.Meter, rs []domain.Reading, es []domain.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meters = append([]domain.Meter(nil), ms...)
	s.readings = append([]domain.Reading(nil), rs...)
	s.events = append([]domain.Event(nil), es...)
	return nil
}

func (s *Store) Meters(context.Context) ([]domain.Meter, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Meter(nil), s.meters...), nil
}

func (s *Store) Readings(context.Context) ([]domain.Reading, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Reading(nil), s.readings...), nil
}

func (s *Store) ReadingsByMeter(_ context.Context, id string) ([]domain.Reading, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.Reading
	for _, r := range s.readings {
		if r.MeterID == id {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out, nil
}

func (s *Store) Events(context.Context) ([]domain.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Event(nil), s.events...), nil
}

func (s *Store) SaveRun(_ context.Context, r domain.AnalysisRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.Steps = append([]domain.StepState(nil), r.Steps...)
	s.runs[r.ID] = r
	return nil
}

func (s *Store) Run(_ context.Context, id string) (domain.AnalysisRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return r, app.ErrNotFound
	}
	return r, nil
}

func (s *Store) LatestRun(context.Context) (domain.AnalysisRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *domain.AnalysisRun
	for _, r := range s.runs {
		r := r
		if latest == nil || r.StartedAt.After(latest.StartedAt) {
			latest = &r
		}
	}
	if latest == nil {
		return domain.AnalysisRun{}, app.ErrNotFound
	}
	return *latest, nil
}

func (s *Store) ReplaceAnomalies(_ context.Context, _ string, as []domain.Anomaly) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.anomalies = append([]domain.Anomaly(nil), as...)
	return nil
}

func (s *Store) Anomalies(context.Context) ([]domain.Anomaly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Anomaly(nil), s.anomalies...), nil
}

func (s *Store) Anomaly(_ context.Context, id string) (domain.Anomaly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.anomalies {
		if a.ID == id {
			return a, nil
		}
	}
	return domain.Anomaly{}, app.ErrNotFound
}

func (s *Store) UpdateAnomalyStatus(_ context.Context, id string, st domain.AnomalyStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.anomalies {
		if s.anomalies[i].ID == id {
			s.anomalies[i].Status = st
			return nil
		}
	}
	return app.ErrNotFound
}
