package explain

import (
	"context"
	"fmt"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/domain"
)

// Template is the deterministic explainer. It is always available and is the
// fallback when the LLM fails or is not configured.
type Template struct{}

func pct(v float64) string          { return analysis.FormatPct(v) }
func num(v float64, dec int) string { return analysis.FormatNumber(v, dec) }
func signal(ev domain.Evidence, code string) (domain.Signal, bool) {
	for _, s := range ev.Signals {
		if s.Code == code {
			return s, true
		}
	}
	return domain.Signal{}, false
}

// Explain implements Explainer.
func (Template) Explain(_ context.Context, a domain.Anomaly) (Explanation, error) {
	ev := a.Evidence
	e := Explanation{Source: "template"}
	switch a.Type {
	case domain.RealAnomaly:
		e.Reason = fmt.Sprintf("Consumo %s por encima del baseline sin evento conocido.", pct(ev.VariationPct))
		if s, ok := signal(ev, "PF_DROP"); ok {
			e.Reason = fmt.Sprintf("Consumo %s por encima del baseline sin evento conocido; %s.", pct(ev.VariationPct), lower(s.Description))
		}
		e.RecommendedAction = "Investigar medidor e instalación."
		e.NextSteps = []string{
			fmt.Sprintf("Inspeccionar en sitio el equipo de %s", ev.MeterName),
			"Revisar arranques en vacío y consumo fuera de turno",
			"Verificar el banco de capacitores y la compensación de reactiva",
		}
	case domain.DataQuality:
		e.Reason = fmt.Sprintf("El consumo es estable (%s), pero %s lecturas son físicamente imposibles; el problema está en la medición.", pct(ev.VariationPct), num(float64(ev.InvalidReadings), 0))
		e.RecommendedAction = "Validar medidor y telemetría."
		e.NextSteps = []string{
			"Revisar transformadores de corriente y de potencial",
			"Validar comunicación y firmware del medidor",
			"Excluir las lecturas inválidas hasta recalibrar",
		}
	case domain.ExplainableAnomaly:
		desc := "un evento operativo registrado"
		if len(ev.RelatedEvents) > 0 {
			desc = lower(ev.RelatedEvents[0].Description)
		}
		e.Reason = fmt.Sprintf("Aumento de %s que coincide con %s; la relación eléctrica se mantiene sana.", pct(ev.VariationPct), desc)
		e.RecommendedAction = "Validar con operación y actualizar el baseline."
		e.NextSteps = []string{"Confirmar con Producción que la carga nueva es la esperada", "Recalcular el baseline con la nueva condición operativa"}
	case domain.FalsePositive:
		desc := "un evento operativo"
		if len(ev.RelatedEvents) > 0 {
			desc = lower(ev.RelatedEvents[0].Description)
		}
		e.Reason = fmt.Sprintf("La variación de %s corresponde a %s y el consumo volvió a su nivel al terminar.", pct(ev.VariationPct), desc)
		e.RecommendedAction = "No escalar."
		e.NextSteps = []string{"Excluir la ventana del evento del cálculo del baseline"}
	}
	return e, nil
}

func lower(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'A' && r[0] <= 'Z' {
		r[0] = r[0] + ('a' - 'A')
	}
	return string(r)
}
