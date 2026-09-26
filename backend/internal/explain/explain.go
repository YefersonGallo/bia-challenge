// Package explain turns the engine's evidence into a human explanation.
//
// The engine decides type, severity, confidence and priority. The explainer
// only writes words: a reason, a recommended action and next steps. Every
// number it writes must exist in the evidence, otherwise the text is rejected
// and the deterministic template is used instead.
package explain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Explanation is the text attached to an anomaly.
type Explanation struct {
	Reason            string   `json:"reason"`
	RecommendedAction string   `json:"recommended_action"`
	NextSteps         []string `json:"next_steps"`
	EvidenceSummary   string   `json:"evidence_summary"`
	Source            string   `json:"source"` // "claude" or "template"
}

// Explainer writes an explanation for a classified anomaly.
type Explainer interface {
	Explain(ctx context.Context, a domain.Anomaly) (Explanation, error)
}

// ErrUngrounded means the text cites numbers that are not in the evidence.
var ErrUngrounded = errors.New("explanation cites numbers that are not in the evidence")

// WithFallback tries the primary explainer and falls back to the secondary
// one on any error (network, timeout, invalid output, ungrounded numbers).
type WithFallback struct {
	Primary, Fallback Explainer
	Timeout           time.Duration // limit for the primary (the plan asks for 10 s); 0 = none
	OnFallback        func(meterID string, err error)
}

// Cached remembers explanations by a hash of the evidence (plus type, severity
// and confidence): running the analysis again with the same data does not call
// the LLM again. Template results are cached too; they are cheap either way.
type Cached struct {
	Next Explainer
	mu   sync.Mutex
	m    map[string]Explanation
}

// Explain implements Explainer.
func (c *Cached) Explain(ctx context.Context, a domain.Anomaly) (Explanation, error) {
	key := evidenceKey(a)
	c.mu.Lock()
	if e, ok := c.m[key]; ok {
		c.mu.Unlock()
		return e, nil
	}
	c.mu.Unlock()
	e, err := c.Next.Explain(ctx, a)
	if err != nil {
		return e, err
	}
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string]Explanation{}
	}
	c.m[key] = e
	c.mu.Unlock()
	return e, nil
}

func evidenceKey(a domain.Anomaly) string {
	b, _ := json.Marshal(struct {
		T domain.AnomalyType
		S domain.Severity
		C float64
		E domain.Evidence
	}{a.Type, a.Severity, a.Confidence, a.Evidence})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Explain implements Explainer.
func (w WithFallback) Explain(ctx context.Context, a domain.Anomaly) (Explanation, error) {
	if w.Primary != nil {
		pctx := ctx
		if w.Timeout > 0 {
			var cancel context.CancelFunc
			pctx, cancel = context.WithTimeout(ctx, w.Timeout)
			defer cancel()
		}
		exp, err := w.Primary.Explain(pctx, a)
		if err == nil {
			if verr := Validate(exp, a.Evidence); verr == nil {
				return exp, nil
			} else {
				err = verr
			}
		}
		if w.OnFallback != nil {
			w.OnFallback(a.MeterID, err)
		}
	}
	return w.Fallback.Explain(ctx, a)
}
