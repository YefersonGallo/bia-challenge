// Package explain turns the engine's evidence into a human explanation.
//
// The engine decides type, severity, confidence and priority. The explainer
// only writes words: a reason, a recommended action and next steps. Every
// number it writes must exist in the evidence, otherwise the text is rejected
// and the deterministic template is used instead.
package explain

import (
	"context"
	"errors"

	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Explanation is the text attached to an anomaly.
type Explanation struct {
	Reason            string   `json:"reason"`
	RecommendedAction string   `json:"recommended_action"`
	NextSteps         []string `json:"next_steps"`
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
	OnFallback        func(meterID string, err error)
}

// Explain implements Explainer.
func (w WithFallback) Explain(ctx context.Context, a domain.Anomaly) (Explanation, error) {
	if w.Primary != nil {
		exp, err := w.Primary.Explain(ctx, a)
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
