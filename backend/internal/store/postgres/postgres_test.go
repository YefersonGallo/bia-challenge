package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
	"github.com/yefersongallo/bia-energy/backend/internal/explain"
	"github.com/yefersongallo/bia-energy/backend/internal/store/postgres"
)

// Integration test: runs only when TEST_DATABASE_URL points to an empty database.
//
//	TEST_DATABASE_URL=postgres://bia:bia@localhost:5432/bia_test?sslmode=disable go test ./internal/store/postgres
func TestPostgresStoreEndToEnd(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if empty, _ := st.Empty(ctx); empty {
		ds := dataset.Generate()
		if err := st.Seed(ctx, ds.Meters, ds.Readings, ds.Events); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := st.Readings(ctx)
	if err != nil || len(rs) != 4032 {
		t.Fatalf("readings = %d, err = %v", len(rs), err)
	}

	svc := app.New(st, analysis.New(analysis.DefaultConfig()), explain.Template{}, app.Options{})
	run, err := svc.StartAnalysis(ctx)
	if err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	run, err = svc.Run(ctx, run.ID)
	if err != nil || run.Status != domain.RunCompleted {
		t.Fatalf("run = %+v, err = %v", run.Status, err)
	}
	as, err := svc.Anomalies(ctx, app.AnomalyQuery{})
	if err != nil || len(as) != 4 || as[0].MeterID != "M-109" {
		t.Fatalf("anomalies = %d (first %v), err = %v", len(as), as, err)
	}
	if len(as[0].Evidence.Signals) == 0 || len(as[0].NextSteps) == 0 {
		t.Fatal("evidence and next steps must round-trip through JSONB")
	}
	if as[0].Impact == nil || len(as[0].ConfidenceBreakdown) == 0 || as[0].ChangePointAt == nil || as[0].EvidenceSummary == "" {
		t.Fatalf("details must round-trip through JSONB: %+v", as[0])
	}
	if _, err := svc.AddAction(ctx, as[0].ID, "tester", app.ActionInput{Action: "note", Note: "revisado en sitio"}); err != nil {
		t.Fatal(err)
	}
	if acts, err := st.Actions(ctx, as[0].ID); err != nil || len(acts) != 1 || acts[0].Note != "revisado en sitio" {
		t.Fatalf("actions = %+v, err = %v", acts, err)
	}
	if _, err := svc.UpdateAnomalyStatus(ctx, as[0].ID, domain.AnomalyAcknowledged); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Anomaly(ctx, as[0].ID)
	if got.Status != domain.AnomalyAcknowledged {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestPostgresUpsertReadings(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if empty, _ := st.Empty(ctx); empty {
		ds := dataset.Generate()
		if err := st.Seed(ctx, ds.Meters, ds.Readings, ds.Events); err != nil {
			t.Fatal(err)
		}
	}
	rs, _ := st.Readings(ctx)
	orig := rs[0]
	changed := orig
	changed.ConsumptionKWh += 1
	fresh := orig
	fresh.Timestamp = time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	added, replaced, err := st.UpsertReadings(ctx, []domain.Reading{changed, fresh})
	if err != nil || added != 1 || replaced != 1 {
		t.Fatalf("added %d replaced %d err %v", added, replaced, err)
	}
	db, _ := sql.Open("postgres", dsn)
	defer db.Close()
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM readings WHERE ts = $1`, fresh.Timestamp)
		_, _, _ = st.UpsertReadings(ctx, []domain.Reading{orig})
	})
	var kwh float64
	if err := db.QueryRowContext(ctx, `SELECT consumption_kwh FROM readings WHERE meter_id = $1 AND ts = $2`, orig.MeterID, orig.Timestamp).Scan(&kwh); err != nil || kwh != changed.ConsumptionKWh {
		t.Fatalf("stored %v, want %v (%v)", kwh, changed.ConsumptionKWh, err)
	}
}
