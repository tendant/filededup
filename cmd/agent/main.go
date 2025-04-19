// cmd/agent/main.go
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/tendant/filededup/pkg/agent"
)

// formatBytes converts bytes to a human-readable string (KB, MB, GB, etc.)
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func main() {
	// Set up structured logging
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	slog.SetDefault(slog.New(logHandler))
	
	// Basic configuration
	dir := flag.String("dir", ".", "Directory to scan")
	server := flag.String("server", "http://localhost:8080", "Server URL")
	machineID := flag.String("machine-id", "default", "Unique machine identifier")
	batchSize := flag.Int("batch", 1000, "Number of files per batch")
	
	// Performance tuning options
	workers := flag.Int("workers", 0, "Number of parallel workers (0 = auto)")
	queueSize := flag.Int("queue-size", 0, "Size of processing queues (0 = auto)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	skipLarge := flag.Bool("skip-large", false, "Skip files larger than the size limit")
	maxSize := flag.Int64("max-size", 1024*1024*1024, "Maximum file size to process in bytes (default 1GB)")
	
	flag.Parse()
	
	// Set log level based on verbose flag
	if *verbose {
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	slog.SetDefault(slog.New(logHandler))
	
	slog.Info("Starting file deduplication agent", 
		"dir", *dir,
		"server", *server,
		"machineID", *machineID,
		"batchSize", *batchSize,
		"skipLarge", *skipLarge,
		"maxSize", formatBytes(*maxSize))

	// Create agent with configuration
	a := agent.New(*dir, *server, *machineID, *batchSize)
	
	// Apply performance tuning if specified
	if *workers > 0 {
		a.WithWorkers(*workers)
	}
	if *queueSize > 0 {
		a.WithQueueSize(*queueSize)
	}
	
	// Configure file size limits
	if *skipLarge {
		a.WithMaxFileSize(*maxSize)
	}
	
	// Run the agent
	if err := a.Run(); err != nil {
		slog.Error("Agent failed", "error", err)
		os.Exit(1)
	}
	
	slog.Info("Agent completed successfully")
}
