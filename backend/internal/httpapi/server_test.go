package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
	"github.com/yefersongallo/bia-energy/backend/internal/explain"
	"github.com/yefersongallo/bia-energy/backend/internal/httpapi"
	"github.com/yefersongallo/bia-energy/backend/internal/store/memory"
)

type harness struct {
	t     *testing.T
	h     http.Handler
	svc   *app.Service
	token string
}

func setup(t *testing.T) *harness {
	t.Helper()
	st := memory.New()
	ds := dataset.Generate()
	_ = st.Seed(context.Background(), ds.Meters, ds.Readings, ds.Events)
	svc := app.New(st, analysis.New(analysis.DefaultConfig()), explain.Template{}, app.Options{})
	auth := httpapi.Auth{Secret: []byte("test"), User: "operador@vatio.demo", Password: "demo", TTL: time.Hour}
	hs := &harness{t: t, h: httpapi.New(svc, httpapi.Config{Auth: auth, CORSOrigin: "*"}), svc: svc}
	var login struct{ Token string }
	hs.do("POST", "/api/auth/login", map[string]string{"email": "operador@vatio.demo", "password": "demo"}, http.StatusOK, &login)
	hs.token = login.Token
	return hs
}

func (hs *harness) do(method, path string, body any, want int, out any) {
	hs.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if hs.token != "" {
		req.Header.Set("Authorization", "Bearer "+hs.token)
	}
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != want {
		hs.t.Fatalf("%s %s = %d, want %d: %s", method, path, rec.Code, want, rec.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			hs.t.Fatalf("decode %s: %v", path, err)
		}
	}
}

func TestAuthRequired(t *testing.T) {
	hs := setup(t)
	hs.token = ""
	hs.do("GET", "/api/meters", nil, http.StatusUnauthorized, nil)
	hs.do("GET", "/api/health", nil, http.StatusOK, nil)
	hs.do("POST", "/api/auth/login", map[string]string{"email": "operador@vatio.demo", "password": "wrong"}, http.StatusUnauthorized, nil)
}

func TestMetersEndpoints(t *testing.T) {
	hs := setup(t)
	var meters []app.MeterSummary
	hs.do("GET", "/api/meters?status=critical", nil, http.StatusOK, &meters)
	if len(meters) != 1 || meters[0].ID != "M-109" {
		t.Fatalf("critical meters = %+v", meters)
	}
	hs.do("GET", "/api/meters?sort=bogus", nil, http.StatusBadRequest, nil)

	var detail app.MeterDetail
	hs.do("GET", "/api/meters/M-104", nil, http.StatusOK, &detail)
	if len(detail.Events) != 1 || len(detail.Stats.Days) != 14 {
		t.Fatalf("M-104 detail: %d events, %d days", len(detail.Events), len(detail.Stats.Days))
	}
	var readings []domain.Reading
	hs.do("GET", "/api/meters/M-104/readings", nil, http.StatusOK, &readings)
	if len(readings) != 336 {
		t.Fatalf("readings = %d, want 336", len(readings))
	}
	hs.do("GET", "/api/meters/M-999", nil, http.StatusNotFound, nil)

	var events []domain.Event
	hs.do("GET", "/api/events", nil, http.StatusOK, &events)
	if len(events) != 2 || events[0].ID != "EV-001" {
		t.Fatalf("events = %+v", events)
	}
}

func TestAnalysisFlowOverHTTP(t *testing.T) {
	hs := setup(t)
	var run domain.AnalysisRun
	hs.do("POST", "/api/ai/analyze", nil, http.StatusAccepted, &run)
	hs.svc.Wait()
	hs.do("GET", "/api/ai/analysis/"+run.ID, nil, http.StatusOK, &run)
	if run.Status != domain.RunCompleted || run.Summary.Anomalies != 4 {
		t.Fatalf("run = %s, summary = %+v", run.Status, run.Summary)
	}
	hs.do("GET", "/api/ai/analysis/latest", nil, http.StatusOK, &run)

	var anomalies []domain.Anomaly
	hs.do("GET", "/api/anomalies", nil, http.StatusOK, &anomalies)
	if anomalies[0].MeterID != "M-109" || anomalies[0].Type != domain.RealAnomaly {
		t.Fatalf("first anomaly = %s %s", anomalies[0].MeterID, anomalies[0].Type)
	}
	var detail map[string]any
	hs.do("GET", "/api/anomalies/"+anomalies[0].ID, nil, http.StatusOK, &detail)
	if detail["anomaly"] != true || detail["recommended_action"] == "" {
		t.Fatalf("detail = %v", detail)
	}
	hs.do("PATCH", "/api/anomalies/"+anomalies[0].ID, map[string]string{"status": "RESOLVED"}, http.StatusOK, nil)
	hs.do("PATCH", "/api/anomalies/"+anomalies[0].ID, map[string]string{"status": "ACKNOWLEDGED"}, http.StatusConflict, nil)

	var summary app.Summary
	hs.do("GET", "/api/dashboard/summary", nil, http.StatusOK, &summary)
	if *summary.OpenAnomalies != 3 {
		t.Fatalf("open anomalies = %d, want 3", *summary.OpenAnomalies)
	}
	hs.do("GET", "/api/reports/latest", nil, http.StatusOK, nil)
}

func TestReportBeforeAnalysisIs404(t *testing.T) {
	setup(t).do("GET", "/api/reports/latest", nil, http.StatusNotFound, nil)
}

func TestCORSPreflight(t *testing.T) {
	hs := setup(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/meters", nil)
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("preflight = %d %v", rec.Code, rec.Header())
	}
}

func TestExpiredToken(t *testing.T) {
	now := time.Now()
	a := httpapi.Auth{Secret: []byte("s"), User: "u", Password: "p", TTL: time.Minute, Now: func() time.Time { return now }}
	tok, _, _ := a.Login("u", "p")
	a.Now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := a.Verify(tok); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err := a.Verify(tok + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestServesSPAWhenStaticDirIsSet(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>vatio</html>"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644)
	hs := setup(t)
	auth := httpapi.Auth{Secret: []byte("test"), User: "u", Password: "p", TTL: time.Hour}
	h := httpapi.New(hs.svc, httpapi.Config{Auth: auth, StaticDir: dir})
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}
	if r := get("/meters/M-109"); r.Code != 200 || !strings.Contains(r.Body.String(), "vatio") {
		t.Fatalf("SPA route = %d %q", r.Code, r.Body.String())
	}
	if r := get("/assets/app.js"); r.Code != 200 || !strings.Contains(r.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset = %d, cache %q", r.Code, r.Header().Get("Cache-Control"))
	}
	if r := get("/api/nope"); r.Code != 401 && r.Code != 404 {
		t.Fatalf("unknown API route = %d", r.Code)
	}
	if r := get("/api/meters"); r.Code != 401 {
		t.Fatalf("API still requires auth, got %d", r.Code)
	}
}

func TestErrorFormat(t *testing.T) {
	hs := setup(t)
	var body struct {
		Error struct{ Code, Message string }
	}
	hs.do("GET", "/api/meters/M-999", nil, http.StatusNotFound, &body)
	if body.Error.Code != "NOT_FOUND" || body.Error.Message == "" {
		t.Fatalf("error body = %+v", body)
	}
	hs.do("GET", "/api/meters?order=sideways", nil, http.StatusBadRequest, &body)
	if body.Error.Code != "INVALID" {
		t.Fatalf("error code = %q", body.Error.Code)
	}
}

func TestReadingsResolutionAndWindow(t *testing.T) {
	hs := setup(t)
	var readings []domain.Reading
	hs.do("GET", "/api/meters/M-104/readings?resolution=raw&from=2026-09-08&to=2026-09-08", nil, http.StatusOK, &readings)
	if len(readings) != 24 {
		t.Fatalf("one day of readings = %d, want 24", len(readings))
	}
	var days []analysis.DayPoint
	hs.do("GET", "/api/meters/M-104/readings?resolution=day&from=2026-09-10T00:00:00Z", nil, http.StatusOK, &days)
	if len(days) == 0 || len(days) >= 14 || days[0].Date < "2026-09-10" {
		t.Fatalf("days from 10th = %d (first %+v)", len(days), days)
	}
	hs.do("GET", "/api/meters/M-104/readings?from=yesterday", nil, http.StatusBadRequest, nil)
	hs.do("GET", "/api/meters/M-104/readings?resolution=minute", nil, http.StatusBadRequest, nil)
}

func TestMetersOrder(t *testing.T) {
	hs := setup(t)
	var desc, asc []app.MeterSummary
	hs.do("GET", "/api/meters?sort=consumption", nil, http.StatusOK, &desc)
	hs.do("GET", "/api/meters?sort=consumption&order=asc", nil, http.StatusOK, &asc)
	if len(desc) != len(asc) || desc[0].ID != asc[len(asc)-1].ID {
		t.Fatalf("asc must reverse desc: %s vs %s", desc[0].ID, asc[len(asc)-1].ID)
	}
}

func TestInsightEndpoints(t *testing.T) {
	hs := setup(t)
	hs.do("POST", "/api/ai/analyze", nil, http.StatusAccepted, nil)
	hs.svc.Wait()

	var heat []app.HeatmapRow
	hs.do("GET", "/api/dashboard/heatmap", nil, http.StatusOK, &heat)
	if len(heat) == 0 || len(heat[0].Days) != 14 {
		t.Fatalf("heatmap rows = %d", len(heat))
	}
	var base app.Baseline
	hs.do("GET", "/api/meters/M-109/baseline", nil, http.StatusOK, &base)
	if base.DailyKWh <= 0 || base.P90[12] < base.P10[12] || base.VoltageBand[0] != 209 {
		t.Fatalf("baseline = %+v", base)
	}
	var fc app.Forecast
	hs.do("GET", "/api/meters/M-109/forecast", nil, http.StatusOK, &fc)
	if len(fc.Points) != 24 || fc.ProjectedKWh <= fc.ExpectedKWh || fc.Impact == nil {
		t.Fatalf("forecast = %d points, projected %.0f vs expected %.0f", len(fc.Points), fc.ProjectedKWh, fc.ExpectedKWh)
	}
	hs.do("GET", "/api/meters/M-999/forecast", nil, http.StatusNotFound, nil)
}

func TestAnomalyActionsRecordTheUser(t *testing.T) {
	hs := setup(t)
	hs.do("POST", "/api/ai/analyze", nil, http.StatusAccepted, nil)
	hs.svc.Wait()
	var anomalies []domain.Anomaly
	hs.do("GET", "/api/anomalies", nil, http.StatusOK, &anomalies)
	id := anomalies[0].ID

	var detail app.AnomalyDetail
	hs.do("POST", "/api/anomalies/"+id+"/actions", map[string]string{"action": "investigate", "note": "cuadrilla asignada"}, http.StatusCreated, &detail)
	if detail.Status != domain.AnomalyInProgress || len(detail.Actions) == 0 {
		t.Fatalf("status = %s, actions = %+v", detail.Status, detail.Actions)
	}
	last := detail.Actions[len(detail.Actions)-1]
	if last.Actor != "operador@vatio.demo" || last.Note != "cuadrilla asignada" {
		t.Fatalf("last action = %+v", last)
	}
	hs.do("POST", "/api/anomalies/"+id+"/actions", map[string]string{"action": "explode"}, http.StatusBadRequest, nil)
	hs.do("POST", "/api/anomalies/"+id+"/actions", map[string]string{}, http.StatusBadRequest, nil)
	hs.do("POST", "/api/anomalies/nope/actions", map[string]string{"action": "note"}, http.StatusNotFound, nil)
}

func TestAnalyzeWhileRunningReturnsTheSameRun(t *testing.T) {
	st := memory.New()
	ds := dataset.Generate()
	_ = st.Seed(context.Background(), ds.Meters, ds.Readings, ds.Events)
	svc := app.New(st, analysis.New(analysis.DefaultConfig()), explain.Template{}, app.Options{StepDelay: 30 * time.Millisecond})
	auth := httpapi.Auth{Secret: []byte("test"), User: "u", Password: "p", TTL: time.Hour}
	hs := &harness{t: t, h: httpapi.New(svc, httpapi.Config{Auth: auth}), svc: svc}
	tok, _, _ := auth.Login("u", "p")
	hs.token = tok
	var first, second domain.AnalysisRun
	hs.do("POST", "/api/ai/analyze", nil, http.StatusAccepted, &first)
	hs.do("POST", "/api/ai/analyze", nil, http.StatusAccepted, &second)
	svc.Wait()
	if first.ID != second.ID {
		t.Fatalf("second call started %s while %s was running", second.ID, first.ID)
	}
}
