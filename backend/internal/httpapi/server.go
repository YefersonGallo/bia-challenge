// Package httpapi exposes the use cases over REST (net/http, Go 1.22 routing).
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Config of the HTTP layer.
type Config struct {
	Auth       Auth
	CORSOrigin string // "*" or a specific origin; empty disables CORS headers
	AIProvider string // shown by /api/health: "claude:<model>" or "template"
	StaticDir  string // when set, the built SPA is served from here (single-container deploys)
	Logger     *slog.Logger
}

type server struct {
	svc *app.Service
	cfg Config
}

// New returns the API handler.
func New(svc *app.Service, cfg Config) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &server{svc: svc, cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("GET /api/dashboard/summary", s.summary)
	mux.HandleFunc("GET /api/meters", s.listMeters)
	mux.HandleFunc("GET /api/meters/{meterId}", s.getMeter)
	mux.HandleFunc("GET /api/meters/{meterId}/readings", s.getReadings)
	mux.HandleFunc("GET /api/meters/{meterId}/events", s.getEvents)
	mux.HandleFunc("GET /api/events", s.listEvents)
	mux.HandleFunc("GET /api/anomalies", s.listAnomalies)
	mux.HandleFunc("GET /api/anomalies/{id}", s.getAnomaly)
	mux.HandleFunc("PATCH /api/anomalies/{id}", s.patchAnomaly)
	mux.HandleFunc("POST /api/ai/analyze", s.analyze)
	mux.HandleFunc("GET /api/ai/analysis/{id}", s.getAnalysis)
	mux.HandleFunc("GET /api/reports/latest", s.report)
	if cfg.StaticDir != "" {
		mux.Handle("GET /", spa(cfg.StaticDir))
	}
	return chain(mux, s.recoverer, s.logRequests, s.cors, s.authenticate)
}

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// --- middleware -------------------------------------------------------------

func (s *server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.cfg.Logger.Error("panic", "path", r.URL.Path, "value", v)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

func (s *server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path != "/api/health" {
			s.cfg.Logger.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
		}
	})
}

func (s *server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.CORSOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.cfg.CORSOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var public = map[string]bool{"/api/health": true, "/api/auth/login": true}

func (s *server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if public[r.URL.Path] || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if _, err := s.cfg.Auth.Verify(token); err != nil {
			writeError(w, http.StatusUnauthorized, "missing or invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// fail maps use-case errors to HTTP status codes.
func (s *server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, app.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.cfg.Logger.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// --- handlers ---------------------------------------------------------------

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	ai := s.cfg.AIProvider
	if ai == "" {
		ai = "template"
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "ai": ai})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email, Password string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	token, exp, ok := s.cfg.Auth.Login(body.Email, body.Password)
	if !ok {
		writeError(w, http.StatusUnauthorized, "credenciales inválidas")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires_at": exp, "user": map[string]string{"email": s.cfg.Auth.User, "name": "Operador demo"}})
}

func (s *server) summary(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.DashboardSummary(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

var validStatus = map[string]bool{"": true, "OK": true, "ALERT": true, "CRITICAL": true}
var validSort = map[string]bool{"": true, "severity": true, "consumption": true, "variation": true}

func (s *server) listMeters(w http.ResponseWriter, r *http.Request) {
	q := app.MeterQuery{Status: strings.ToUpper(r.URL.Query().Get("status")), Q: r.URL.Query().Get("q"), Sort: r.URL.Query().Get("sort")}
	if !validStatus[q.Status] || !validSort[q.Sort] {
		writeError(w, http.StatusBadRequest, "status must be OK|ALERT|CRITICAL and sort severity|consumption|variation")
		return
	}
	out, err := s.svc.ListMeters(r.Context(), q)
	if err != nil {
		s.fail(w, err)
		return
	}
	if out == nil {
		out = []app.MeterSummary{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getMeter(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Meter(r.Context(), r.PathValue("meterId"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getReadings(w http.ResponseWriter, r *http.Request) {
	bucket := r.URL.Query().Get("bucket")
	if bucket != "" && bucket != "hour" && bucket != "day" {
		writeError(w, http.StatusBadRequest, "bucket must be hour or day")
		return
	}
	out, err := s.svc.Readings(r.Context(), r.PathValue("meterId"), bucket)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) listEvents(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Events(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getEvents(w http.ResponseWriter, r *http.Request) {
	if _, err := s.svc.Meter(r.Context(), r.PathValue("meterId")); err != nil {
		s.fail(w, err)
		return
	}
	out, err := s.svc.MeterEvents(r.Context(), r.PathValue("meterId"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) listAnomalies(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Anomalies(r.Context(), app.AnomalyQuery{Type: r.URL.Query().Get("type"), Severity: r.URL.Query().Get("severity")})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) getAnomaly(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Anomaly(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) patchAnomaly(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status domain.AnomalyStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
		writeError(w, http.StatusBadRequest, "body must be {\"status\": \"ACKNOWLEDGED|IN_PROGRESS|RESOLVED\"}")
		return
	}
	out, err := s.svc.UpdateAnomalyStatus(r.Context(), r.PathValue("id"), body.Status)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) analyze(w http.ResponseWriter, r *http.Request) {
	run, err := s.svc.StartAnalysis(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Location", "/api/ai/analysis/"+run.ID)
	writeJSON(w, http.StatusAccepted, run)
}

func (s *server) getAnalysis(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Run(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) report(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.LatestReport(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
