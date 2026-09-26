package explain

import (
	"context"
	"fmt"
	"strings"

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
			"Revisar cargas nuevas, conexiones no autorizadas o fallas",
			"Evaluar el impacto del factor de potencia bajo y la compensación de reactiva",
		}
	case domain.DataQuality:
		what := "lecturas físicamente inconsistentes"
		if _, ok := signal(ev, "VOLTAGE_JUMPS"); ok {
			what = "lecturas físicamente inconsistentes (saltos de voltaje y de factor de potencia que no cuadran con el consumo)"
		}
		e.Reason = fmt.Sprintf("El consumo es estable (%s), pero hay %s %s; el problema está en la medición, no en la carga.", pct(ev.VariationPct), num(float64(ev.InvalidReadings), 0), what)
		e.RecommendedAction = "Validar medidor y telemetría."
		e.NextSteps = []string{
			"Validar el medidor y los transformadores de corriente y de potencial",
			"Marcar sus lecturas como no confiables para facturación y reportes",
			"Revisar comunicación y firmware del medidor",
		}
	case domain.ExplainableAnomaly:
		e.Reason = fmt.Sprintf("Aumento de %s que coincide con %s; la relación eléctrica se mantiene sana.", pct(ev.VariationPct), eventRef(ev))
		e.RecommendedAction = "Validar con operación y actualizar el baseline."
		e.NextSteps = []string{
			"Confirmar con Producción que la carga nueva es la esperada",
			"Ajustar el baseline, la potencia contratada y la compensación de reactiva si aplica",
		}
	case domain.FalsePositive:
		e.Reason = fmt.Sprintf("La caída de %s durante %s h coincide con %s, y el consumo volvió a su nivel al terminar.", pct(ev.ShiftPct), num(float64(ev.PersistentHours), 0), eventRef(ev))
		e.RecommendedAction = "No escalar."
		e.NextSteps = []string{"Registrar como evento operativo conocido", "Excluir la ventana del evento del cálculo del baseline"}
	}
	e.EvidenceSummary = summary(ev)
	return e, nil
}

// summary lists the strongest evidence signals in one line.
func summary(ev domain.Evidence) string {
	var parts []string
	for _, s := range ev.Signals {
		if len(parts) == 3 {
			break
		}
		parts = append(parts, s.Description)
	}
	return strings.Join(parts, " · ")
}

// eventRef names the first related event as it was registered (quoted: the
// dataset may describe events in another language).
func eventRef(ev domain.Evidence) string {
	if len(ev.RelatedEvents) == 0 {
		return "un evento operativo registrado"
	}
	return fmt.Sprintf("el evento registrado «%s»", ev.RelatedEvents[0].Description)
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
