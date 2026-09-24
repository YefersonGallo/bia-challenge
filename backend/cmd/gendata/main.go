// Command gendata writes the deterministic synthetic dataset to a directory.
//
//	go run ./cmd/gendata -out ./data
package main

import (
	"flag"
	"log"

	"github.com/yefersongallo/bia-energy/backend/internal/dataset"
	"github.com/yefersongallo/bia-energy/backend/internal/ingest"
)

func main() {
	out := flag.String("out", "data", "output directory")
	flag.Parse()
	ds := dataset.Generate()
	if err := ingest.WriteDir(*out, ingest.Data{Meters: ds.Meters, Readings: ds.Readings, Events: ds.Events}); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s to %s", ds.Describe(), *out)
}
