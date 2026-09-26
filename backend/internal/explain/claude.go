package explain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Claude writes explanations with the Anthropic Messages API. It forces a
// tool call so the answer always comes back as structured JSON.
type Claude struct {
	APIKey  string
	Model   string
	BaseURL string // defaults to https://api.anthropic.com
	HTTP    *http.Client
}

// NewClaude builds a client with a sane timeout.
func NewClaude(apiKey, model string) *Claude {
	return &Claude{APIKey: apiKey, Model: model, BaseURL: "https://api.anthropic.com", HTTP: &http.Client{Timeout: 20 * time.Second}}
}

const systemPrompt = `Eres el analista de energía de una planta industrial. Recibes la evidencia que calculó un motor de análisis determinístico sobre un medidor eléctrico.
Reglas:
- Escribe en español, claro y directo, para un jefe de mantenimiento.
- No cambies el tipo, la severidad ni la confianza: ya fueron decididos por el motor.
- Usa solo cifras que aparezcan tal cual en la evidencia. No calcules cifras nuevas (sumas, restas, promedios, conteos, horas entre fechas) ni inventes números, fechas o causas.
- Una caída se escribe con el valor absoluto ("caída de 79,9 %"), nunca con doble signo.
- "reason": una o dos frases con la conclusión y las cifras clave.
- "evidence_summary": una frase con las 2 o 3 evidencias numéricas más fuertes.
- "next_steps": de 1 a 4 pasos verificables, coherentes con la acción ya decidida ("recommended_action").
- La acción recomendada la decide el motor; no la cambies ni propongas otra distinta.
- Si citas una cifra derivada (kWh extra, costo, reactiva), usa la que viene en "impact" o en "extra_kwh_per_day".
- Las descripciones de los eventos pueden venir en inglés: tradúcelas o cítalas entre comillas, sin agregar datos.
- Los días se cuentan desde el inicio del periodo (día 1 a día 14); no inventes fechas de calendario.
- La magnitud del cambio es "shift_pct" (desvío medio del episodio, con su signo: negativo es una caída). "variation_pct" es solo el estado de las últimas 24 h; en un falso positivo o una parada ya recuperada no lo presentes como el cambio.`

type toolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // a string, or content blocks for a tool result
}

type request struct {
	Model      string            `json:"model"`
	MaxTokens  int               `json:"max_tokens"`
	System     string            `json:"system"`
	Tools      []toolSchema      `json:"tools"`
	ToolChoice map[string]string `json:"tool_choice"`
	Messages   []message         `json:"messages"`
}

type response struct {
	Content []struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

var explanationTool = toolSchema{
	Name:        "report_explanation",
	Description: "Entrega la explicación de la anomalía para el operador.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason":           map[string]any{"type": "string"},
			"evidence_summary": map[string]any{"type": "string"},
			"next_steps":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 4},
		},
		"required": []string{"reason", "evidence_summary", "next_steps"},
	},
}

// Explain implements Explainer. When the answer cites a number that is not in
// the evidence, the model gets one chance to rewrite it, told which number failed.
func (c *Claude) Explain(ctx context.Context, a domain.Anomaly) (Explanation, error) {
	e, raw, err := c.call(ctx, a, nil)
	if err == nil {
		err = Validate(e, a)
	}
	if !errors.Is(err, ErrUngrounded) || raw == nil {
		return e, err
	}
	retry := []message{
		{Role: "assistant", Content: []map[string]any{{"type": "tool_use", "id": raw.id, "name": explanationTool.Name, "input": raw.input}}},
		{Role: "user", Content: []map[string]any{{"type": "tool_result", "tool_use_id": raw.id, "is_error": true,
			"content": "Rechazado: " + err.Error() + ". Reescribe la explicación citando solo cifras que aparezcan tal cual en la evidencia, sin calcular ninguna."}}},
	}
	e, _, err = c.call(ctx, a, retry)
	return e, err
}

type toolCall struct {
	id    string
	input json.RawMessage
}

func (c *Claude) call(ctx context.Context, a domain.Anomaly, follow []message) (Explanation, *toolCall, error) {
	payload, err := json.Marshal(map[string]any{
		"type": a.Type, "severity": a.Severity, "confidence": a.Confidence,
		"recommended_action": Action(a.Type),
		"extra_kwh_per_day":  round1(a.Evidence.CurrentKWh - a.Evidence.BaselineKWh),
		"impact":             a.Impact,
		"evidence":           a.Evidence,
	})
	if err != nil {
		return Explanation{}, nil, err
	}
	msgs := append([]message{{Role: "user", Content: "Evidencia del análisis (JSON):\n" + string(payload)}}, follow...)
	body, _ := json.Marshal(request{
		Model: c.Model, MaxTokens: 800, System: systemPrompt,
		Tools: []toolSchema{explanationTool}, ToolChoice: map[string]string{"type": "tool", "name": explanationTool.Name},
		Messages: msgs,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Explanation{}, nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Explanation{}, nil, fmt.Errorf("claude request: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return Explanation{}, nil, fmt.Errorf("claude status %d: %s", res.StatusCode, string(raw))
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return Explanation{}, nil, fmt.Errorf("claude decode: %w", err)
	}
	for _, block := range r.Content {
		if block.Type != "tool_use" || block.Name != explanationTool.Name {
			continue
		}
		var e Explanation
		if err := json.Unmarshal(block.Input, &e); err != nil {
			return Explanation{}, nil, fmt.Errorf("claude tool input: %w", err)
		}
		e.Source = "claude"
		e.RecommendedAction = Action(a.Type) // decided by the engine, never by the model
		return e, &toolCall{id: block.ID, input: block.Input}, nil
	}
	return Explanation{}, nil, fmt.Errorf("claude returned no tool_use block")
}
