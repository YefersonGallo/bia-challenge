package explain

import (
	"context"
	"fmt"
	"math"
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

// Action is the recommended action of each type. The engine decides it, like
// the type and the severity; an LLM only writes the words around it.
func Action(t domain.AnomalyType) string {
	switch t {
	case domain.RealAnomaly:
		return "Investigar medidor e instalación."
	case domain.DataQuality:
		return "Validar medidor y telemetría."
	case domain.ExplainableAnomaly:
		return "Validar con operación y actualizar el baseline."
	case domain.FalsePositive:
		return "No escalar."
	}
	return "Revisar el medidor."
}

// abs formats a percentage without sign: the words already say the direction.
func abs(v float64) string { return num(math.Abs(v), 1) + "%" }

// Explain implements Explainer.
func (Template) Explain(_ context.Context, a domain.Anomaly) (Explanation, error) {
	ev := a.Evidence
	e := Explanation{Source: "template", RecommendedAction: Action(a.Type)}
	// The direction comes from the episode (ShiftPct); the 24 h variation can
	// point the other way once a drop has recovered.
	down := ev.ShiftPct < 0 || (ev.ShiftPct == 0 && ev.VariationPct < 0)
	size := ev.VariationPct
	if down || (ev.ShiftPct != 0 && math.Signbit(ev.ShiftPct) != math.Signbit(ev.VariationPct)) {
		size = ev.ShiftPct
	}
	noEvent := "sin evento conocido"
	if len(ev.RelatedEvents) > 0 {
		noEvent = "sin un evento que lo explique"
	}
	switch a.Type {
	case domain.RealAnomaly:
		dir := "por encima"
		if down {
			dir = "por debajo"
		}
		e.Reason = fmt.Sprintf("Consumo %s %s del baseline %s.", abs(size), dir, noEvent)
		if s, ok := signal(ev, "PF_DROP"); ok {
			e.Reason = fmt.Sprintf("Consumo %s %s del baseline %s; %s.", abs(size), dir, noEvent, lower(s.Description))
		}
		e.NextSteps = []string{
			fmt.Sprintf("Inspeccionar en sitio el equipo de %s", ev.MeterName),
			"Revisar cargas nuevas, conexiones no autorizadas o fallas",
		}
		if _, ok := signal(ev, "PF_DROP"); ok {
			e.NextSteps = append(e.NextSteps, "Evaluar el impacto del factor de potencia bajo y la compensación de reactiva")
		}
	case domain.DataQuality:
		what := "lecturas físicamente inconsistentes"
		if _, ok := signal(ev, "VOLTAGE_JUMPS"); ok {
			what = "lecturas físicamente inconsistentes (saltos de voltaje y de factor de potencia que no cuadran con el consumo)"
		}
		count := float64(ev.InvalidReadings)
		if count == 0 { // only gaps or repeated hours
			m, _ := signal(ev, "MISSING_HOURS")
			d, _ := signal(ev, "DUPLICATE_READINGS")
			count, what = m.Value+d.Value, "horas faltantes o repetidas"
		}
		if _, stable := signal(ev, "STABLE_CONSUMPTION"); stable {
			e.Reason = fmt.Sprintf("El consumo es estable (%s), pero hay %s %s; el problema está en la medición, no en la carga.", pct(ev.VariationPct), num(count, 0), what)
		} else {
			e.Reason = fmt.Sprintf("Hay %s %s; antes de interpretar la variación del consumo (%s) hay que validar la medición.", num(count, 0), what, pct(ev.VariationPct))
		}
		e.NextSteps = []string{
			"Validar el medidor y los transformadores de corriente y de potencial",
			"Marcar sus lecturas como no confiables para facturación y reportes",
			"Revisar comunicación y firmware del medidor",
		}
	case domain.ExplainableAnomaly:
		change := "Aumento"
		if down {
			change = "Caída"
		}
		electrical := "la relación eléctrica se mantiene sana"
		if s, ok := signal(ev, "PF_DROP"); ok {
			electrical = lower(s.Description) + ", a revisar aunque el cambio de carga esté explicado"
		}
		e.Reason = fmt.Sprintf("%s de %s que coincide con %s; %s.", change, abs(size), eventRef(ev), electrical)
		e.NextSteps = []string{
			"Confirmar con Producción que el cambio de carga es el esperado",
			"Ajustar el baseline, la potencia contratada y la compensación de reactiva si aplica",
		}
	case domain.FalsePositive:
		change := "La caída"
		if !down {
			change = "El aumento"
		}
		e.Reason = fmt.Sprintf("%s de %s durante %s h coincide con %s, y el consumo volvió a su nivel al terminar.", change, abs(ev.ShiftPct), num(float64(ev.PersistentHours), 0), eventRef(ev))
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

// eventRef names the event that explains the change (the engine lists it
// first), as it was registered: quoted, since the dataset may use another language.
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
