// Package httpapi exposes the use cases over REST (net/http, Go 1.22 routing).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
	"github.com/yefersongallo/bia-energy/backend/internal/live"
)

// Config of the HTTP layer.
type Config struct {
	Auth       Auth
	CORSOrigin string     // "*" or a specific origin; empty disables CORS headers
	AIProvider string     // shown by /api/health: "claude:<model>" or "template"
	StaticDir  string     // when set, the built SPA is served from here (single-container deploys)
	Live       LiveStream // optional: enables /api/stream (hourly replay with SSE)
	Logger     *slog.Logger
}

// LiveStream is the port of the streaming replay (implemented by live.Runner).
type LiveStream interface {
	Subscribe(lastID uint64) (*live.Snapshot, uint64, []live.Message, chan live.Message, func())
	State() live.State
	Alerts() []live.Alert
	Control(action string, speed float64) (live.State, bool)
}

type server struct {
	svc    *app.Service
	cfg    Config
	logins *limiter // failed logins per client
}

// MaxBody caps request bodies: every JSON body of this API is tiny.
const MaxBody = 1 << 20

// New returns the API handler.
func New(svc *app.Service, cfg Config) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &server{svc: svc, cfg: cfg, logins: newLimiter(10, 5*time.Minute)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("GET /api/dashboard/summary", s.summary)
	mux.HandleFunc("GET /api/dashboard/heatmap", s.heatmap)
	mux.HandleFunc("GET /api/meters", s.listMeters)
	mux.HandleFunc("GET /api/meters/{meterId}", s.getMeter)
	mux.HandleFunc("GET /api/meters/{meterId}/readings", s.getReadings)
	mux.HandleFunc("GET /api/meters/{meterId}/events", s.getEvents)
	mux.HandleFunc("GET /api/meters/{meterId}/baseline", s.baseline)
	mux.HandleFunc("GET /api/meters/{meterId}/forecast", s.forecast)
	mux.HandleFunc("GET /api/events", s.listEvents)
	mux.HandleFunc("GET /api/anomalies", s.listAnomalies)
	mux.HandleFunc("GET /api/anomalies/{id}", s.getAnomaly)
	mux.HandleFunc("PATCH /api/anomalies/{id}", s.patchAnomaly)
	mux.HandleFunc("POST /api/anomalies/{id}/actions", s.postAction)
	mux.HandleFunc("POST /api/ai/analyze", s.analyze)
	mux.HandleFunc("GET /api/ai/analysis/{id}", s.getAnalysis)
	mux.HandleFunc("GET /api/reports/latest", s.report)
	if cfg.Live != nil {
		mux.HandleFunc("GET /api/stream", s.stream)
		mux.HandleFunc("POST /api/stream/token", s.streamToken)
		mux.HandleFunc("GET /api/stream/state", s.streamState)
		mux.HandleFunc("POST /api/stream/control", s.streamControl)
	}
	var root http.Handler = mux
	if cfg.StaticDir != "" {
		// The SPA lives in its own router: a catch-all "GET /" next to the API
		// routes would answer GET /api/ai/analyze with index.html instead of 405.
		app := spa(cfg.StaticDir)
		root = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
				mux.ServeHTTP(w, r)
				return
			}
			app.ServeHTTP(w, r)
		})
	}
	return chain(root, s.recoverer, s.logRequests, securityHeaders, limitBody, jsonErrors, s.cors, s.authenticate)
}

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// --- middleware -------------------------------------------------------------

// securityHeaders applies the same headers nginx adds, so the single-container
// deploy (the API serving the SPA) is protected too.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > MaxBody {
			writeError(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "request body too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
		next.ServeHTTP(w, r)
	})
}

// jsonErrors turns the router's plain-text 404/405 into the API error format.
func jsonErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api" && !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&errRewriter{ResponseWriter: w}, r)
	})
}

type errRewriter struct {
	http.ResponseWriter
	rewrote bool
}

func (w *errRewriter) WriteHeader(code int) {
	if (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) && strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		w.rewrote = true
		kind, msg := "NOT_FOUND", "no such API route"
		if code == http.StatusMethodNotAllowed {
			kind, msg = "METHOD_NOT_ALLOWED", "method not allowed for this route"
		}
		writeError(w.ResponseWriter, code, kind, msg)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *errRewriter) Write(b []byte) (int, error) {
	if w.rewrote {
		return len(b), nil // the router's text body is replaced by the JSON one
	}
	return w.ResponseWriter.Write(b)
}

func (w *errRewriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.cfg.Logger.Error("panic", "path", r.URL.Path, "value", v)
				writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
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

// Flush lets the SSE handler push events through the logging middleware.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

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
		// EventSource cannot send headers: the stream takes a token in the URL, and
		// only a short-lived stream token (POST /api/stream/token), so a URL that
		// ends up in an access log is useless a minute later.
		streamURL := token == "" && r.URL.Path == "/api/stream"
		if streamURL {
			token = r.URL.Query().Get("token")
		}
		user, err := s.cfg.Auth.Verify(token)
		if err == nil && strings.HasPrefix(user, streamPrefix) != streamURL {
			err = errBadToken // a stream token opens only the stream, and the stream only takes one
		}
		user = strings.TrimPrefix(user, streamPrefix)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}

type userKey struct{}

// userOf returns the authenticated user of the request (the token subject).
func userOf(r *http.Request) string {
	if u, ok := r.Context().Value(userKey{}).(string); ok && u != "" {
		return u
	}
	return "unknown"
}

// --- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiError is the body of every error response: {"error": {"code", "message"}}.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]apiError{"error": {Code: code, Message: msg}})
}

// fail maps use-case errors to HTTP status codes.
func (s *server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, app.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, app.ErrConflict):
		writeError(w, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, app.ErrInvalid):
		writeError(w, http.StatusBadRequest, "INVALID", err.Error())
	default:
		s.cfg.Logger.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal error")
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
	client := clientIP(r)
	if s.logins.blocked(client) {
		writeError(w, http.StatusTooManyRequests, "TOO_MANY_ATTEMPTS", "demasiados intentos; espera unos minutos")
		return
	}
	var body struct{ Email, Password string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID", "invalid JSON body")
		return
	}
	token, exp, ok := s.cfg.Auth.Login(body.Email, body.Password)
	if !ok {
		s.logins.fail(client)
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "credenciales inválidas")
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
var validOrder = map[string]bool{"": true, "asc": true, "desc": true}

func (s *server) listMeters(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	q := app.MeterQuery{Status: strings.ToUpper(v.Get("status")), Q: v.Get("q"), Sort: v.Get("sort"), Order: strings.ToLower(v.Get("order"))}
	if !validStatus[q.Status] || !validSort[q.Sort] || !validOrder[q.Order] {
		writeError(w, http.StatusBadRequest, "INVALID", "status must be OK|ALERT|CRITICAL, sort severity|consumption|variation and order asc|desc")
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
	v := r.URL.Query()
	q := app.ReadingsQuery{Bucket: v.Get("bucket")}
	if res := v.Get("resolution"); res != "" { // alias used by the plan: raw | hour | day
		q.Bucket = res
	}
	if q.Bucket == "raw" {
		q.Bucket = ""
	}
	if q.Bucket != "" && q.Bucket != "hour" && q.Bucket != "day" {
		writeError(w, http.StatusBadRequest, "INVALID", "resolution must be raw, hour or day")
		return
	}
	var err error
	if q.From, err = parseTime(v.Get("from"), false); err == nil {
		q.To, err = parseTime(v.Get("to"), true)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID", "from and to must be RFC 3339 timestamps or YYYY-MM-DD dates")
		return
	}
	out, err := s.svc.Readings(r.Context(), r.PathValue("meterId"), q)
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
		writeError(w, http.StatusBadRequest, "INVALID", "body must be {\"status\": \"ACKNOWLEDGED|IN_PROGRESS|RESOLVED\"}")
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

// parseTime accepts RFC 3339 or a date; a date used as upper bound covers the whole day.
func parseTime(v string, end bool) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, err
	}
	if end {
		t = t.Add(24*time.Hour - time.Second)
	}
	return &t, nil
}

func (s *server) heatmap(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Heatmap(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) baseline(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Baseline(r.Context(), r.PathValue("meterId"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) forecast(w http.ResponseWriter, r *http.Request) {
	out, err := s.svc.Forecast(r.Context(), r.PathValue("meterId"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) postAction(w http.ResponseWriter, r *http.Request) {
	var body app.ActionInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Action == "" {
		writeError(w, http.StatusBadRequest, "INVALID", `body must be {"action": "acknowledge|investigate|validate|resolve|dismiss|note", "note": "…"}`)
		return
	}
	out, err := s.svc.AddAction(r.Context(), r.PathValue("id"), userOf(r), body)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
