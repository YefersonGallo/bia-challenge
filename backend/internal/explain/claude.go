package explain

import (
	"bytes"
	"context"
	"encoding/json"
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
- Usa solo cifras que aparezcan en la evidencia. No inventes números, fechas ni causas no respaldadas.
- "reason": una o dos frases con la conclusión y las cifras clave.
- "recommended_action": una acción concreta y corta.
- "next_steps": de 1 a 4 pasos verificables.
- Las descripciones de los eventos pueden venir en inglés: tradúcelas o cítalas entre comillas, sin agregar datos.
- Los días se cuentan desde el inicio del periodo (día 1 a día 14); no inventes fechas de calendario.`

type toolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
			"reason":             map[string]any{"type": "string"},
			"recommended_action": map[string]any{"type": "string"},
			"next_steps":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 4},
		},
		"required": []string{"reason", "recommended_action", "next_steps"},
	},
}

// Explain implements Explainer.
func (c *Claude) Explain(ctx context.Context, a domain.Anomaly) (Explanation, error) {
	payload, err := json.Marshal(map[string]any{
		"type": a.Type, "severity": a.Severity, "confidence": a.Confidence, "evidence": a.Evidence,
	})
	if err != nil {
		return Explanation{}, err
	}
	body, _ := json.Marshal(request{
		Model: c.Model, MaxTokens: 800, System: systemPrompt,
		Tools: []toolSchema{explanationTool}, ToolChoice: map[string]string{"type": "tool", "name": explanationTool.Name},
		Messages: []message{{Role: "user", Content: "Evidencia del análisis (JSON):\n" + string(payload)}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Explanation{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Explanation{}, fmt.Errorf("claude request: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return Explanation{}, fmt.Errorf("claude status %d: %s", res.StatusCode, string(raw))
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return Explanation{}, fmt.Errorf("claude decode: %w", err)
	}
	for _, block := range r.Content {
		if block.Type != "tool_use" || block.Name != explanationTool.Name {
			continue
		}
		var e Explanation
		if err := json.Unmarshal(block.Input, &e); err != nil {
			return Explanation{}, fmt.Errorf("claude tool input: %w", err)
		}
		e.Source = "claude"
		return e, nil
	}
	return Explanation{}, fmt.Errorf("claude returned no tool_use block")
}
