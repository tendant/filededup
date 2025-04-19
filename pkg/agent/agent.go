// internal/agent/agent.go
package agent

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type FileRecord struct {
	MachineID string    `json:"machine_id"`
	Path      string    `json:"path"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	MTime     time.Time `json:"mtime"`
	Hash      string    `json:"hash"`
}

type Agent struct {
	RootDir    string
	ServerURL  string
	MachineID  string
	BatchSize  int
	NumWorkers int // Number of parallel workers
}

func New(root, server, machineID string, batch int) *Agent {
	// Default to number of CPUs for worker count
	numWorkers := runtime.NumCPU()
	
	return &Agent{
		RootDir:    root,
		ServerURL:  strings.TrimRight(server, "/"),
		MachineID:  machineID,
		BatchSize:  batch,
		NumWorkers: numWorkers,
	}
}

func (a *Agent) Run() error {
	// Initialize progress tracking
	var processedFiles, totalFiles, totalBytes, queuedFiles atomic.Int64
	var startTime = time.Now()
	
	// First, count total files to process
	slog.Info("Counting files to process...")
	filepath.Walk(a.RootDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalFiles.Add(1)
			totalBytes.Add(info.Size())
		}
		return nil
	})
	slog.Info("Starting file scan", 
		"totalFiles", totalFiles.Load(), 
		"totalBytes", formatBytes(totalBytes.Load()),
		"workers", a.NumWorkers)
	
	// Start progress reporting in a separate goroutine
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				processed := processedFiles.Load()
				total := totalFiles.Load()
				queued := queuedFiles.Load()
				if total > 0 {
					progress := float64(processed) / float64(total) * 100
					elapsed := time.Since(startTime)
					var eta time.Duration
					if processed > 0 {
						eta = time.Duration(float64(elapsed) / float64(processed) * float64(total-processed))
					}
					slog.Info("Scan progress", 
						"processed", processed,
						"queued", queued,
						"total", total,
						"percent", fmt.Sprintf("%.1f%%", progress),
						"elapsed", elapsed.Round(time.Second),
						"eta", eta.Round(time.Second))
				}
			case <-progressDone:
				return
			}
		}
	}()

	// Create channels for the worker pool
	fileQueue := make(chan string, 1000) // Buffer to avoid blocking
	resultQueue := make(chan FileRecord, 1000)
	batchQueue := make(chan []FileRecord, 10)
	
	// Create a WaitGroup to wait for all workers to finish
	var wg sync.WaitGroup
	
	// Start file processing workers
	slog.Info("Starting file processing workers", "count", a.NumWorkers)
	for i := 0; i < a.NumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for path := range fileQueue {
				// Process the file
				info, err := os.Stat(path)
				if err != nil || info.IsDir() {
					processedFiles.Add(1)
					continue
				}
				
				// Hash the file
				hash, err := hashFile(path)
				if err != nil {
					processedFiles.Add(1)
					continue
				}
				
				// Get the absolute directory path
				dirPath := filepath.Dir(path)
				absPath, err := filepath.Abs(dirPath)
				if err != nil {
					slog.Error("Failed to get absolute path", "path", dirPath, "error", err)
					absPath = dirPath // Fallback to the original path
				}
				filename := filepath.Base(path)
				
				// Create a file record and send it to the result queue
				resultQueue <- FileRecord{
					MachineID: a.MachineID,
					Path:      absPath,
					Filename:  filename,
					Size:      info.Size(),
					MTime:     info.ModTime(),
					Hash:      hash,
				}
				
				// Update progress
				processedFiles.Add(1)
			}
		}(i)
	}
	
	// Start batch processing worker
	batchDone := make(chan struct{})
	go func() {
		defer close(batchDone)
		for batch := range batchQueue {
			if err := a.sendBatch(batch); err != nil {
				slog.Error("Failed to send batch", "error", err)
			}
		}
	}()
	
	// Start result collector
	resultDone := make(chan struct{})
	go func() {
		defer close(resultDone)
		batch := make([]FileRecord, 0, a.BatchSize)
		
		for record := range resultQueue {
			batch = append(batch, record)
			
			if len(batch) >= a.BatchSize {
				batchQueue <- batch
				batch = make([]FileRecord, 0, a.BatchSize)
			}
		}
		
		// Send any remaining records
		if len(batch) > 0 {
			batchQueue <- batch
		}
	}()
	
	// Walk the directory and queue files
	err := filepath.Walk(a.RootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		
		// Queue the file for processing
		fileQueue <- path
		queuedFiles.Add(1)
		
		return nil
	})

	// Close the file queue to signal workers to finish
	close(fileQueue)
	
	// Wait for all file processing workers to finish
	wg.Wait()
	
	// Close the result queue to signal the result collector to finish
	close(resultQueue)
	
	// Wait for the result collector to finish
	<-resultDone
	
	// Close the batch queue to signal the batch processor to finish
	close(batchQueue)
	
	// Wait for the batch processor to finish
	<-batchDone
	
	// Signal the progress goroutine to stop
	close(progressDone)
	
	if err != nil {
		return fmt.Errorf("walk error: %w", err)
	}
	
	// Print final summary
	elapsed := time.Since(startTime)
	slog.Info("Scan completed", 
		"totalFiles", processedFiles.Load(),
		"totalBytes", formatBytes(totalBytes.Load()),
		"duration", elapsed.Round(time.Second),
		"filesPerSecond", float64(processedFiles.Load())/elapsed.Seconds(),
		"workers", a.NumWorkers)
	
	return nil
}

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

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (a *Agent) sendBatch(batch []FileRecord) error {
	slog.Info("Sending batch of files", "count", len(batch))
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(batch); err != nil {
		slog.Error("Failed to encode batch", "error", err)
		return fmt.Errorf("failed to encode batch: %w", err)
	}
	zw.Close()

	req, err := http.NewRequest("POST", a.ServerURL+"/files", &buf)
	if err != nil {
		slog.Error("Failed to create request", "error", err)
		return fmt.Errorf("request creation error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("HTTP request failed", "error", err)
		return fmt.Errorf("http error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		slog.Error("Unexpected server response", "status", resp.Status)
		return fmt.Errorf("server responded with: %s", resp.Status)
	}
	slog.Info("Batch sent successfully")
	return nil
}
