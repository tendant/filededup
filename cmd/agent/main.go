// cmd/agent/main.go
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/tendant/filededup/pkg/agent"
)

func main() {
	// Set up structured logging
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(logHandler))
	
	dir := flag.String("dir", ".", "Directory to scan")
	server := flag.String("server", "http://localhost:8080", "Server URL")
	machineID := flag.String("machine-id", "default", "Unique machine identifier")
	batchSize := flag.Int("batch", 1000, "Number of files per batch")
	flag.Parse()
	
	slog.Info("Starting file deduplication agent", 
		"dir", *dir,
		"server", *server,
		"machineID", *machineID,
		"batchSize", *batchSize)

	a := agent.New(*dir, *server, *machineID, *batchSize)
	if err := a.Run(); err != nil {
		slog.Error("Agent failed", "error", err)
		os.Exit(1)
	}
	
	slog.Info("Agent completed successfully")
}
