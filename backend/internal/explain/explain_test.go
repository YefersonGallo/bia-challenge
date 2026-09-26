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
		if err := Validate(e, a); err != nil {
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
	if err := Validate(e, a); !errors.Is(err, ErrUngrounded) {
		t.Fatalf("expected ErrUngrounded, got %v", err)
	}
}

func TestValidateAcceptsSpanishFormattedEvidence(t *testing.T) {
	a := findings(t)["M-109"]
	reason := "Consumo " + analysis.FormatPct(a.Evidence.VariationPct) + " sobre el baseline de " + analysis.FormatNumber(a.Evidence.BaselineKWh, 0) + " kWh desde el día 8."
	if err := Validate(Explanation{Reason: reason, RecommendedAction: "Investigar medidor e instalación."}, a); err != nil {
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
		input, _ := json.Marshal(map[string]any{"reason": reason, "recommended_action": "Detener el compresor de inmediato", "next_steps": []string{"Inspeccionar el compresor"}})
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
		if err := Validate(e, a); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

// The action is decided by the engine per type: whatever the model writes there
// is replaced (AC-06 expects the same four actions with or without Claude).
func TestClaudeCannotChangeTheRecommendedAction(t *testing.T) {
	for id, a := range findings(t) {
		srv := claudeServer(t, "Consumo "+analysis.FormatPct(a.Evidence.VariationPct)+".", http.StatusOK)
		c := NewClaude("test-key", "test-model")
		c.BaseURL = srv.URL
		e, err := c.Explain(context.Background(), a)
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		if e.RecommendedAction != Action(a.Type) {
			t.Errorf("%s action = %q, want %q", id, e.RecommendedAction, Action(a.Type))
		}
	}
}

// Figures derived from the evidence and sent to the model (extra kWh per day,
// projected impact) are valid citations, rounded or not.
func TestValidateAcceptsDerivedFigures(t *testing.T) {
	a := findings(t)["M-109"]
	extra := a.Evidence.CurrentKWh - a.Evidence.BaselineKWh
	texts := []string{
		"Consume " + analysis.FormatNumber(extra, 1) + " kWh/día de más.",
		"Consume " + analysis.FormatNumber(extra, 0) + " kWh/día de más.",
		"Unos " + analysis.FormatNumber(a.Impact.ExtraKWhPerMonth, 0) + " kWh al mes, $" + analysis.FormatNumber(a.Impact.CostPerMonthCOP, 0) + " COP.",
	}
	for _, r := range texts {
		if err := Validate(Explanation{Reason: r, RecommendedAction: Action(a.Type)}, a); err != nil {
			t.Errorf("%q rejected: %v", r, err)
		}
	}
}

type fixedExplainer struct {
	e     Explanation
	calls int
}

func (f *fixedExplainer) Explain(context.Context, domain.Anomaly) (Explanation, error) {
	f.calls++
	return f.e, nil
}

// A rejected answer is not cached: the next analysis asks again.
func TestCacheDoesNotKeepRejectedAnswers(t *testing.T) {
	a := findings(t)["M-109"]
	inner := &fixedExplainer{e: Explanation{Reason: "Fuga de 9.999 kWh.", RecommendedAction: Action(a.Type), Source: "claude"}}
	c := &Cached{Next: inner}
	for i := 0; i < 2; i++ {
		if _, err := c.Explain(context.Background(), a); !errors.Is(err, ErrUngrounded) {
			t.Fatalf("err = %v", err)
		}
	}
	if inner.calls != 2 {
		t.Fatalf("calls = %d, want 2 (no caching of rejected answers)", inner.calls)
	}
}

// The template follows the direction of the change: never "Aumento de −79%".
func TestTemplateWordsFollowTheDirection(t *testing.T) {
	down := domain.Anomaly{Type: domain.RealAnomaly, Evidence: domain.Evidence{MeterName: "Horno", VariationPct: 1.1, ShiftPct: -79.8}}
	e, _ := Template{}.Explain(context.Background(), down)
	if !strings.Contains(e.Reason, "79,8% por debajo") || strings.Contains(e.Reason, "+") {
		t.Errorf("real drop: %q", e.Reason)
	}
	expl := domain.Anomaly{Type: domain.ExplainableAnomaly, Evidence: domain.Evidence{VariationPct: -79.8, ShiftPct: -79.8}}
	e, _ = Template{}.Explain(context.Background(), expl)
	if !strings.HasPrefix(e.Reason, "Caída de 79,8%") {
		t.Errorf("explainable drop: %q", e.Reason)
	}
	fp := domain.Anomaly{Type: domain.FalsePositive, Evidence: domain.Evidence{ShiftPct: -79.9, PersistentHours: 12}}
	e, _ = Template{}.Explain(context.Background(), fp)
	if !strings.HasPrefix(e.Reason, "La caída de 79,9% durante 12 h") {
		t.Errorf("false positive: %q", e.Reason)
	}
}
