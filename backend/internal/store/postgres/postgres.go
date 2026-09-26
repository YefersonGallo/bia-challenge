// Package postgres implements app.Store on PostgreSQL (database/sql + lib/pq).
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/lib/pq"

	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store is the PostgreSQL adapter.
type Store struct{ db *sql.DB }

var _ app.Store = (*Store)(nil)

// Open connects, pings and runs pending migrations.
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the pool.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, f).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrations.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: %w", f, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, f); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Empty(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM readings`).Scan(&n)
	return n == 0, err
}

// Seed loads meters, readings (with COPY) and events in one transaction.
func (s *Store) Seed(ctx context.Context, ms []domain.Meter, rs []domain.Reading, es []domain.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, m := range ms {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meters (id, name, location, created_at) VALUES ($1,$2,$3,$4) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, location = EXCLUDED.location`, m.ID, m.Name, m.Location, m.CreatedAt); err != nil {
			return err
		}
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn("readings", "meter_id", "ts", "consumption_kwh", "voltage_v", "current_a", "power_factor", "status"))
	if err != nil {
		return err
	}
	for _, r := range rs {
		if _, err := stmt.ExecContext(ctx, r.MeterID, r.Timestamp, r.ConsumptionKWh, r.VoltageV, r.CurrentA, r.PowerFactor, r.Status); err != nil {
			return err
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		return err
	}
	if err := stmt.Close(); err != nil {
		return err
	}
	for _, e := range es {
		if _, err := tx.ExecContext(ctx, `INSERT INTO events (id, meter_id, ts, type, description) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`, e.ID, e.MeterID, e.Timestamp, e.Type, e.Description); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Meters(ctx context.Context) ([]domain.Meter, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, location, created_at FROM meters ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Meter
	for rows.Next() {
		var m domain.Meter
		if err := rows.Scan(&m.ID, &m.Name, &m.Location, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.CreatedAt = m.CreatedAt.UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) readings(ctx context.Context, where string, args ...any) ([]domain.Reading, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT meter_id, ts, consumption_kwh, voltage_v, current_a, power_factor, status FROM readings `+where+` ORDER BY meter_id, ts`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Reading
	for rows.Next() {
		var r domain.Reading
		if err := rows.Scan(&r.MeterID, &r.Timestamp, &r.ConsumptionKWh, &r.VoltageV, &r.CurrentA, &r.PowerFactor, &r.Status); err != nil {
			return nil, err
		}
		r.Timestamp = r.Timestamp.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Readings(ctx context.Context) ([]domain.Reading, error) { return s.readings(ctx, "") }

func (s *Store) ReadingsByMeter(ctx context.Context, id string) ([]domain.Reading, error) {
	return s.readings(ctx, "WHERE meter_id = $1", id)
}

func (s *Store) Events(ctx context.Context) ([]domain.Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, meter_id, ts, type, description FROM events ORDER BY ts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		if err := rows.Scan(&e.ID, &e.MeterID, &e.Timestamp, &e.Type, &e.Description); err != nil {
			return nil, err
		}
		e.Timestamp = e.Timestamp.UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) SaveRun(ctx context.Context, r domain.AnalysisRun) error {
	steps, _ := json.Marshal(r.Steps)
	var summary []byte
	if r.Summary != nil {
		summary, _ = json.Marshal(r.Summary)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO analysis_runs (id, status, started_at, finished_at, current_step, steps, summary, error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET status = EXCLUDED.status, finished_at = EXCLUDED.finished_at,
			current_step = EXCLUDED.current_step, steps = EXCLUDED.steps, summary = EXCLUDED.summary, error = EXCLUDED.error`,
		r.ID, r.Status, r.StartedAt, r.FinishedAt, r.CurrentStep, steps, nullableJSON(summary), r.Error)
	return err
}

func nullableJSON(b []byte) any {
	if b == nil {
		return nil
	}
	return b
}

func scanRun(row interface{ Scan(...any) error }) (domain.AnalysisRun, error) {
	var r domain.AnalysisRun
	var steps, summary []byte
	var finished sql.NullTime
	if err := row.Scan(&r.ID, &r.Status, &r.StartedAt, &finished, &r.CurrentStep, &steps, &summary, &r.Error); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r, app.ErrNotFound
		}
		return r, err
	}
	r.StartedAt = r.StartedAt.UTC()
	if finished.Valid {
		t := finished.Time.UTC()
		r.FinishedAt = &t
	}
	if err := json.Unmarshal(steps, &r.Steps); err != nil {
		return r, err
	}
	if len(summary) > 0 {
		r.Summary = &domain.RunSummary{}
		if err := json.Unmarshal(summary, r.Summary); err != nil {
			return r, err
		}
	}
	return r, nil
}

const runCols = `id, status, started_at, finished_at, current_step, steps, summary, error`

func (s *Store) Run(ctx context.Context, id string) (domain.AnalysisRun, error) {
	return scanRun(s.db.QueryRowContext(ctx, `SELECT `+runCols+` FROM analysis_runs WHERE id = $1`, id))
}

func (s *Store) LatestRun(ctx context.Context) (domain.AnalysisRun, error) {
	return scanRun(s.db.QueryRowContext(ctx, `SELECT `+runCols+` FROM analysis_runs ORDER BY started_at DESC LIMIT 1`))
}

func (s *Store) ReplaceAnomalies(ctx context.Context, runID string, as []domain.Anomaly) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM anomalies WHERE analysis_id = $1`, runID); err != nil {
		return err
	}
	for _, a := range as {
		steps, _ := json.Marshal(a.NextSteps)
		ev, _ := json.Marshal(a.Evidence)
		det, _ := json.Marshal(detailsOf(a))
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO anomalies (id, analysis_id, meter_id, detected_at, type, severity, confidence, priority_score, rank,
				reason, recommended_action, next_steps, explained_by, status, evidence, details)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			a.ID, runID, a.MeterID, a.DetectedAt, a.Type, a.Severity, a.Confidence, a.PriorityScore, a.Rank,
			a.Reason, a.RecommendedAction, steps, a.ExplainedBy, a.Status, ev, det); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const anomalyCols = `id, analysis_id, meter_id, detected_at, type, severity, confidence, priority_score, rank, reason, recommended_action, next_steps, explained_by, status, evidence, details`

// details groups the anomaly fields added by the plan (migration 002).
type details struct {
	EvidenceSummary     string                       `json:"evidence_summary"`
	ConfidenceBreakdown []domain.ConfidenceComponent `json:"confidence_breakdown"`
	Impact              *domain.Impact               `json:"projected_impact,omitempty"`
	ChangePointAt       *time.Time                   `json:"change_point_at,omitempty"`
	EndedAt             *time.Time                   `json:"ended_at,omitempty"`
}

func detailsOf(a domain.Anomaly) details {
	return details{a.EvidenceSummary, a.ConfidenceBreakdown, a.Impact, a.ChangePointAt, a.EndedAt}
}

func scanAnomaly(row interface{ Scan(...any) error }) (domain.Anomaly, error) {
	var a domain.Anomaly
	var steps, ev, det []byte
	if err := row.Scan(&a.ID, &a.AnalysisID, &a.MeterID, &a.DetectedAt, &a.Type, &a.Severity, &a.Confidence, &a.PriorityScore, &a.Rank,
		&a.Reason, &a.RecommendedAction, &steps, &a.ExplainedBy, &a.Status, &ev, &det); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return a, app.ErrNotFound
		}
		return a, err
	}
	a.DetectedAt = a.DetectedAt.UTC()
	if err := json.Unmarshal(steps, &a.NextSteps); err != nil {
		return a, err
	}
	var d details
	if err := json.Unmarshal(det, &d); err != nil {
		return a, err
	}
	a.EvidenceSummary, a.ConfidenceBreakdown, a.Impact, a.ChangePointAt, a.EndedAt = d.EvidenceSummary, d.ConfidenceBreakdown, d.Impact, d.ChangePointAt, d.EndedAt
	return a, json.Unmarshal(ev, &a.Evidence)
}

// AddAction implements app.ActionRepository.
func (s *Store) AddAction(ctx context.Context, a domain.AnomalyAction) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO anomaly_actions (id, anomaly_id, action, note, status, actor, at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		a.ID, a.AnomalyID, a.Action, a.Note, string(a.Status), a.Actor, a.At)
	return err
}

// Actions implements app.ActionRepository (oldest first).
func (s *Store) Actions(ctx context.Context, anomalyID string) ([]domain.AnomalyAction, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, anomaly_id, action, note, status, actor, at FROM anomaly_actions WHERE anomaly_id = $1 ORDER BY at, id`, anomalyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AnomalyAction
	for rows.Next() {
		var a domain.AnomalyAction
		var st string
		if err := rows.Scan(&a.ID, &a.AnomalyID, &a.Action, &a.Note, &st, &a.Actor, &a.At); err != nil {
			return nil, err
		}
		a.Status, a.At = domain.AnomalyStatus(st), a.At.UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}

// Anomalies returns the findings of the most recent run that has any.
func (s *Store) Anomalies(ctx context.Context) ([]domain.Anomaly, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+anomalyCols+` FROM anomalies
		WHERE analysis_id = (
			SELECT r.id FROM analysis_runs r WHERE EXISTS (SELECT 1 FROM anomalies a WHERE a.analysis_id = r.id)
			ORDER BY r.started_at DESC LIMIT 1)
		ORDER BY rank`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Anomaly
	for rows.Next() {
		a, err := scanAnomaly(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Anomaly(ctx context.Context, id string) (domain.Anomaly, error) {
	return scanAnomaly(s.db.QueryRowContext(ctx, `SELECT `+anomalyCols+` FROM anomalies WHERE id = $1`, id))
}

func (s *Store) UpdateAnomalyStatus(ctx context.Context, id string, st domain.AnomalyStatus) error {
	res, err := s.db.ExecContext(ctx, `UPDATE anomalies SET status = $2 WHERE id = $1`, id, st)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return app.ErrNotFound
	}
	return nil
}
