// Command api runs the Vatio REST API.
//
// Configuration (environment):
//
//	PORT               default 8080
//	DATABASE_URL       PostgreSQL DSN; when empty an in-memory store is used
//	DATA_DIR           directory with readings.csv / events.csv (default ./data)
//	ANTHROPIC_API_KEY  enables explanations written by Claude (alias: LLM_API_KEY)
//	ANTHROPIC_MODEL    Claude model id (default claude-sonnet-5; alias: LLM_MODEL)
//	LLM_TIMEOUT_MS     limit for one Claude call before the template is used (default 20000)
//	TARIFF_COP_PER_KWH energy tariff for the impact estimate (default 850)
//	AUTH_SECRET        HMAC secret for tokens (required when DATABASE_URL is set)
//	DEMO_USER / DEMO_PASSWORD  demo credentials
//	CORS_ORIGIN        origin allowed to call the API from another site (default: none; the SPA is same-origin)
//	STEP_DELAY_MS      pause between pipeline steps for the UI (default 450)
//	STATIC_DIR         serve the built SPA from this directory (single-container deploys)
//	DB_FALLBACK_MEMORY "true" keeps the app up with the in-memory store if PostgreSQL is unreachable
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/app"
	"github.com/yefersongallo/bia-energy/backend/internal/explain"
	"github.com/yefersongallo/bia-energy/backend/internal/httpapi"
	"github.com/yefersongallo/bia-energy/backend/internal/ingest"
	"github.com/yefersongallo/bia-energy/backend/internal/store/memory"
	"github.com/yefersongallo/bia-energy/backend/internal/store/postgres"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	// "api healthcheck" lets distroless containers probe the server without a shell.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := openStore(ctx, log)
	if err != nil {
		return err
	}
	if err := seed(ctx, store, env("DATA_DIR", "data"), log); err != nil {
		return err
	}

	var explainer explain.Explainer = explain.Template{}
	aiProvider := "template"
	if key := env("ANTHROPIC_API_KEY", os.Getenv("LLM_API_KEY")); key != "" {
		model := env("ANTHROPIC_MODEL", env("LLM_MODEL", "claude-sonnet-5"))
		timeout, _ := strconv.Atoi(env("LLM_TIMEOUT_MS", "20000"))
		aiProvider = "claude:" + model
		// Only Claude's answers are cached: a failed call is retried on the next run.
		explainer = explain.WithFallback{
			Primary: &explain.Cached{Next: explain.NewClaude(key, model)}, Fallback: explain.Template{},
			Timeout:    time.Duration(timeout) * time.Millisecond,
			OnFallback: func(meter string, err error) { log.Warn("claude fallback to template", "meter", meter, "err", err) },
			OnSuccess: func(meter string, took time.Duration) {
				log.Info("claude explanation", "meter", meter, "ms", took.Milliseconds())
			},
		}
		log.Info("explanations by Claude enabled", "model", model, "timeout_ms", timeout)
	} else {
		log.Info("ANTHROPIC_API_KEY not set: using template explanations")
	}

	cfg := analysis.DefaultConfig()
	if v, err := strconv.ParseFloat(os.Getenv("TARIFF_COP_PER_KWH"), 64); err == nil && v > 0 {
		cfg.TariffCOPPerKWh = v
	}
	delay, _ := strconv.Atoi(env("STEP_DELAY_MS", "450"))
	svc := app.New(store, analysis.New(cfg), explainer, app.Options{StepDelay: time.Duration(delay) * time.Millisecond, Logger: log})

	if err := svc.RecoverInterrupted(ctx); err != nil {
		log.Warn("could not recover an interrupted analysis", "err", err)
	}

	secret := os.Getenv("AUTH_SECRET")
	if secret == "" && os.Getenv("DATABASE_URL") != "" {
		return errors.New("AUTH_SECRET is required when DATABASE_URL is set (generate one with: openssl rand -hex 32)")
	}
	if secret == "" {
		secret = "dev-secret-change-me"
		log.Warn("AUTH_SECRET not set: using an insecure development secret")
	}
	storeKind := "memory"
	if _, ok := store.(*postgres.Store); ok {
		storeKind = "postgres"
	}
	handler := httpapi.New(svc, httpapi.Config{AIProvider: aiProvider, StoreKind: storeKind, StaticDir: os.Getenv("STATIC_DIR"),
		Auth:       httpapi.Auth{Secret: []byte(secret), User: env("DEMO_USER", "operador@vatio.demo"), Password: env("DEMO_PASSWORD", "demo"), TTL: 12 * time.Hour},
		CORSOrigin: os.Getenv("CORS_ORIGIN"),
		Logger:     log,
	})
	srv := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdown)
	svc.Wait()
	return err
}

func openStore(ctx context.Context, log *slog.Logger) (app.Store, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Info("DATABASE_URL not set: using in-memory store")
		return memory.New(), nil
	}
	var lastErr error
	for i := 0; i < 20; i++ { // wait for the database container
		st, err := postgres.Open(ctx, dsn)
		if err == nil {
			log.Info("connected to PostgreSQL")
			return st, nil
		}
		lastErr = err
		// Managed databases on a private network often run without TLS, while
		// lib/pq defaults to sslmode=require: retry without TLS in that case.
		if strings.Contains(err.Error(), "SSL is not enabled") && !strings.Contains(dsn, "sslmode=") {
			dsn = withParam(dsn, "sslmode=disable")
			log.Info("PostgreSQL has no TLS: retrying with sslmode=disable")
			continue
		}
		time.Sleep(time.Second)
	}
	// Free databases can expire (Render deletes them after 30 days): a demo
	// deploy may opt into keeping the app up with the in-memory store.
	if os.Getenv("DB_FALLBACK_MEMORY") == "true" {
		log.Warn("PostgreSQL unreachable: falling back to the in-memory store", "err", lastErr)
		return memory.New(), nil
	}
	return nil, lastErr
}

func withParam(dsn, kv string) string {
	if strings.Contains(dsn, "?") {
		return dsn + "&" + kv
	}
	return dsn + "?" + kv
}

func seed(ctx context.Context, store app.Store, dir string, log *slog.Logger) error {
	empty, err := store.Empty(ctx)
	if err != nil || !empty {
		return err
	}
	data, err := ingest.LoadDir(dir)
	if err != nil {
		return err
	}
	log.Info("seeding", "dir", dir, "meters", len(data.Meters), "readings", len(data.Readings), "events", len(data.Events))
	return store.Seed(ctx, data.Meters, data.Readings, data.Events)
}

func healthcheck() int {
	c := http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://127.0.0.1:" + env("PORT", "8080") + "/api/health")
	if err != nil {
		return 1
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
