// Command analyze runs the anomaly engine over a data directory and prints the
// findings in the output format of the challenge (one JSON object per anomaly),
// using the deterministic template explanations. Handy to check a dataset
// without starting the API:
//
//	go run ./cmd/analyze -data ./data
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
	flag.Parse()

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
		x, _ := explain.Template{}.Explain(context.Background(), f.Anomaly)
		out := map[string]any{
			"meter_id": f.Anomaly.MeterID, "anomaly": f.Anomaly.IsAnomaly(), "type": f.Anomaly.Type, "severity": f.Anomaly.Severity,
			"confidence": f.Anomaly.Confidence, "reason": x.Reason, "recommended_action": x.RecommendedAction,
		}
		if *verbose {
			out["priority"] = f.Anomaly.Rank
			out["signals"] = f.Anomaly.Evidence.Signals
		}
		_ = enc.Encode(out)
	}
}
