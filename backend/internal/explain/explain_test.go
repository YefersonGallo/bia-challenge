package explain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

func findings(t *testing.T) map[string]domain.Anomaly {
	t.Helper()
	ds := dataset.Generate()
	res := analysis.New(analysis.DefaultConfig()).Analyze(analysis.Input{Meters: ds.Meters, Readings: ds.Readings, Events: ds.Events}, nil)
	out := map[string]domain.Anomaly{}
	for _, f := range res.Findings {
		out[f.Anomaly.MeterID] = f.Anomaly
	}
	return out
}

func TestTemplateIsGroundedForEveryFinding(t *testing.T) {
	for id, a := range findings(t) {
		e, err := Template{}.Explain(context.Background(), a)
		if err != nil {
			t.Fatal(err)
		}
		if err := Validate(e, a.Evidence); err != nil {
			t.Errorf("%s: template not grounded: %v (%s)", id, err, e.Reason)
		}
	}
}

func TestTemplateActions(t *testing.T) {
	f := findings(t)
	want := map[string]string{"M-109": "Investigar medidor e instalación.", "M-112": "Validar medidor y telemetría.", "M-106": "No escalar."}
	for id, action := range want {
		e, _ := Template{}.Explain(context.Background(), f[id])
		if e.RecommendedAction != action {
			t.Errorf("%s action = %q, want %q", id, e.RecommendedAction, action)
		}
	}
}

func TestValidateRejectsInventedNumbers(t *testing.T) {
	a := findings(t)["M-109"]
	e := Explanation{Reason: "El consumo subió 250% por una fuga de 37 horas.", RecommendedAction: "Investigar."}
	if err := Validate(e, a.Evidence); !errors.Is(err, ErrUngrounded) {
		t.Fatalf("expected ErrUngrounded, got %v", err)
	}
}

func TestValidateAcceptsSpanishFormattedEvidence(t *testing.T) {
	a := findings(t)["M-109"]
	reason := "Consumo " + analysis.FormatPct(a.Evidence.VariationPct) + " sobre el baseline de " + analysis.FormatNumber(a.Evidence.BaselineKWh, 0) + " kWh desde el día 8."
	if err := Validate(Explanation{Reason: reason, RecommendedAction: "Investigar medidor e instalación."}, a.Evidence); err != nil {
		t.Fatalf("grounded text rejected: %v (%s)", err, reason)
	}
}

func claudeServer(t *testing.T, reason string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing auth headers")
		}
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ToolChoice["name"] != "report_explanation" || !strings.Contains(req.Messages[0].Content, "evidence") {
			t.Errorf("unexpected request: %+v", req.ToolChoice)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
			return
		}
		input, _ := json.Marshal(map[string]any{"reason": reason, "recommended_action": "Investigar medidor e instalación.", "next_steps": []string{"Inspeccionar el compresor"}})
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []map[string]any{{"type": "tool_use", "name": "report_explanation", "input": json.RawMessage(input)}}})
	}))
}

func TestClaudeExplainerParsesToolUse(t *testing.T) {
	a := findings(t)["M-109"]
	srv := claudeServer(t, "Consumo "+analysis.FormatPct(a.Evidence.VariationPct)+" sin evento conocido.", http.StatusOK)
	defer srv.Close()
	c := NewClaude("test-key", "test-model")
	c.BaseURL = srv.URL
	e, err := WithFallback{Primary: c, Fallback: Template{}}.Explain(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if e.Source != "claude" {
		t.Fatalf("source = %s, want claude", e.Source)
	}
}

func TestFallbackOnUngroundedClaudeOutput(t *testing.T) {
	a := findings(t)["M-109"]
	srv := claudeServer(t, "Consumo +300% por una fuga de 45 A.", http.StatusOK)
	defer srv.Close()
	c := NewClaude("test-key", "test-model")
	c.BaseURL = srv.URL
	var fellBack error
	e, err := WithFallback{Primary: c, Fallback: Template{}, OnFallback: func(_ string, err error) { fellBack = err }}.Explain(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if e.Source != "template" || !errors.Is(fellBack, ErrUngrounded) {
		t.Fatalf("expected template fallback for ungrounded text, got %s (%v)", e.Source, fellBack)
	}
}

func TestFallbackOnAPIError(t *testing.T) {
	a := findings(t)["M-112"]
	srv := claudeServer(t, "", http.StatusServiceUnavailable)
	defer srv.Close()
	c := NewClaude("test-key", "test-model")
	c.BaseURL = srv.URL
	e, err := WithFallback{Primary: c, Fallback: Template{}}.Explain(context.Background(), a)
	if err != nil || e.Source != "template" {
		t.Fatalf("expected template fallback, got %v %v", e.Source, err)
	}
}

type countingExplainer struct {
	calls int
	delay time.Duration
}

func (c *countingExplainer) Explain(ctx context.Context, a domain.Anomaly) (Explanation, error) {
	c.calls++
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return Explanation{}, ctx.Err()
		}
	}
	return Template{}.Explain(ctx, a)
}

// Running the analysis twice with the same evidence must not call the LLM again.
func TestCachedExplainerReusesExplanations(t *testing.T) {
	a := findings(t)["M-109"]
	inner := &countingExplainer{}
	c := &Cached{Next: inner}
	for i := 0; i < 3; i++ {
		if _, err := c.Explain(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("calls = %d, want 1", inner.calls)
	}
	b := a
	b.Confidence += 0.01
	_, _ = c.Explain(context.Background(), b)
	if inner.calls != 2 {
		t.Fatalf("changed evidence must call again, calls = %d", inner.calls)
	}
}

// A slow LLM falls back to the template instead of blocking the analysis.
func TestFallbackOnTimeout(t *testing.T) {
	a := findings(t)["M-104"]
	var fell error
	w := WithFallback{Primary: &countingExplainer{delay: time.Second}, Fallback: Template{}, Timeout: 20 * time.Millisecond,
		OnFallback: func(_ string, err error) { fell = err }}
	start := time.Now()
	e, err := w.Explain(context.Background(), a)
	if err != nil || e.Source != "template" || fell == nil {
		t.Fatalf("explanation %+v, err %v, fallback %v", e, err, fell)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("timeout not applied")
	}
}

func TestTemplateWritesAGroundedSummary(t *testing.T) {
	for id, a := range findings(t) {
		e, _ := Template{}.Explain(context.Background(), a)
		if e.EvidenceSummary == "" {
			t.Errorf("%s without evidence summary", id)
		}
		if err := Validate(e, a.Evidence); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}
