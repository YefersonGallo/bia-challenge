// Command analyze runs the anomaly engine over a data directory and prints the
// findings in the output format of the challenge (one JSON object per anomaly).
// Handy to check a dataset, or the Claude connection, without starting the API:
//
//	go run ./cmd/analyze -data ./data            # template explanations
//	ANTHROPIC_API_KEY=… go run ./cmd/analyze -llm # explanations written by Claude
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/yefersongallo/bia-energy/backend/internal/analysis"
	"github.com/yefersongallo/bia-energy/backend/internal/explain"
	"github.com/yefersongallo/bia-energy/backend/internal/ingest"
)

func main() {
	dir := flag.String("data", "data", "directory with readings.csv and events.csv")
	verbose := flag.Bool("v", false, "also print the rule status of every meter and the evidence signals")
	llm := flag.Bool("llm", false, "write the explanations with Claude (needs ANTHROPIC_API_KEY)")
	flag.Parse()

	var explainer explain.Explainer = explain.Template{}
	if *llm {
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			log.Fatal("-llm needs ANTHROPIC_API_KEY")
		}
		model := os.Getenv("ANTHROPIC_MODEL")
		if model == "" {
			model = "claude-sonnet-5"
		}
		fmt.Fprintf(os.Stderr, "explainer    Claude · %s\n", model)
		explainer = explain.WithFallback{
			Primary: explain.NewClaude(key, model), Fallback: explain.Template{},
			OnFallback: func(meter string, err error) { fmt.Fprintf(os.Stderr, "FALLBACK     %s: %v\n", meter, err) },
		}
	}

	d, err := ingest.LoadDir(*dir)
	if err != nil {
		log.Fatal(err)
	}
	eng := analysis.New(analysis.DefaultConfig())
	res := eng.Analyze(analysis.Input{Meters: d.Meters, Readings: d.Readings, Events: d.Events}, func(step, result string) {
		fmt.Fprintf(os.Stderr, "%-12s %s\n", step, result)
	})
	if *verbose {
		for _, m := range d.Meters {
			s := res.Stats[m.ID]
			fmt.Fprintf(os.Stderr, "%s %-8s baseline %7.1f · 24 h %7.1f · %6.1f%% · %s\n", m.ID, s.Status, s.BaselineKWh, s.CurrentKWh, s.VariationPct, s.StatusReason)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	for _, f := range res.Findings {
		x, err := explainer.Explain(context.Background(), f.Anomaly)
		if err != nil {
			log.Fatalf("%s: %v", f.Anomaly.MeterID, err)
		}
		out := map[string]any{
			"meter_id": f.Anomaly.MeterID, "anomaly": f.Anomaly.IsAnomaly(), "type": f.Anomaly.Type, "severity": f.Anomaly.Severity,
			"confidence": f.Anomaly.Confidence, "reason": x.Reason, "recommended_action": x.RecommendedAction,
		}
		if *llm || *verbose {
			out["explained_by"] = x.Source
		}
		if *verbose {
			out["priority"] = f.Anomaly.Rank
			out["signals"] = f.Anomaly.Evidence.Signals
		}
		_ = enc.Encode(out)
	}
}
