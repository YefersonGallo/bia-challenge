package postgres_test

import (
	"context"
	"os"
	"testing"

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
	if _, err := svc.UpdateAnomalyStatus(ctx, as[0].ID, domain.AnomalyAcknowledged); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Anomaly(ctx, as[0].ID)
	if got.Status != domain.AnomalyAcknowledged {
		t.Fatalf("status = %s", got.Status)
	}
}
