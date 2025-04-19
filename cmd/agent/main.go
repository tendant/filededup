// cmd/agent/main.go
package main

import (
	"flag"
	"log"

	"github.com/tendant/filededup/pkg/agent"
)

func main() {
	dir := flag.String("dir", ".", "Directory to scan")
	server := flag.String("server", "http://localhost:8080", "Server URL")
	machineID := flag.String("machine-id", "default", "Unique machine identifier")
	batchSize := flag.Int("batch", 1000, "Number of files per batch")
	flag.Parse()

	a := agent.New(*dir, *server, *machineID, *batchSize)
	if err := a.Run(); err != nil {
		log.Fatalf("Agent failed: %v", err)
	}
}
