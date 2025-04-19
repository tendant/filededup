// internal/agent/agent.go
package agent

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
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
	NumWorkers int // Number of parallel workers for file processing
	QueueSize  int // Size of the internal processing queues
}

// New creates a new Agent with the specified parameters
func New(root, server, machineID string, batch int) *Agent {
	// Default to number of CPUs for worker count
	numWorkers := runtime.NumCPU()
	
	// For very large directories, we might want more workers than CPUs
	// to better handle I/O-bound operations
	if numWorkers < 4 {
		numWorkers = 4 // Minimum 4 workers
	}
	
	// Default queue size based on batch size
	queueSize := batch * 2
	if queueSize < 1000 {
		queueSize = 1000 // Minimum queue size
	}
	
	return &Agent{
		RootDir:    root,
		ServerURL:  strings.TrimRight(server, "/"),
		MachineID:  machineID,
		BatchSize:  batch,
		NumWorkers: numWorkers,
		QueueSize:  queueSize,
	}
}

// WithWorkers sets the number of parallel workers
func (a *Agent) WithWorkers(workers int) *Agent {
	if workers > 0 {
		a.NumWorkers = workers
	}
	return a
}

// WithQueueSize sets the size of internal processing queues
func (a *Agent) WithQueueSize(size int) *Agent {
	if size > 0 {
		a.QueueSize = size
	}
	return a
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
		"workers", a.NumWorkers,
		"queueSize", a.QueueSize,
		"batchSize", a.BatchSize)
	
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

	// Create channels for the worker pool with appropriate buffer sizes
	fileQueue := make(chan string, a.QueueSize)
	resultQueue := make(chan FileRecord, a.QueueSize)
	batchQueue := make(chan []FileRecord, a.NumWorkers) // One batch per worker
	
	// Create a WaitGroup to wait for all workers to finish
	var wg sync.WaitGroup
	
	// Start file processing workers with adaptive behavior
	slog.Info("Starting file processing workers", "count", a.NumWorkers)
	
	// Create a semaphore to limit concurrent file operations
	// This helps prevent overwhelming the file system with too many open files
	fileSemaphore := make(chan struct{}, a.NumWorkers*2)
	
	for i := 0; i < a.NumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for path := range fileQueue {
				// Acquire semaphore before file operations
				fileSemaphore <- struct{}{}
				
				// Process the file
				info, err := os.Stat(path)
				if err != nil || info.IsDir() {
					processedFiles.Add(1)
					<-fileSemaphore // Release semaphore
					continue
				}
				
				// Optimize for file size - use different strategies for small vs large files
				var hash string
				if info.Size() < 10*1024*1024 { // 10MB threshold
					// For small files, hash the entire file
					hash, err = hashFile(path)
				} else {
					// For large files, use a faster sampling approach
					hash, err = hashLargeFile(path, info.Size())
				}
				
				if err != nil {
					processedFiles.Add(1)
					<-fileSemaphore // Release semaphore
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
				
				// Release semaphore after file operations
				<-fileSemaphore
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
	// Calculate performance metrics
	filesPerSecond := float64(processedFiles.Load()) / elapsed.Seconds()
	bytesPerSecond := float64(totalBytes.Load()) / elapsed.Seconds()
	
	slog.Info("Scan completed", 
		"totalFiles", processedFiles.Load(),
		"totalBytes", formatBytes(totalBytes.Load()),
		"duration", elapsed.Round(time.Second),
		"filesPerSecond", fmt.Sprintf("%.1f", filesPerSecond),
		"throughput", formatBytes(int64(bytesPerSecond))+"/s",
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

// hashLargeFile efficiently hashes large files by sampling portions of the file
// rather than reading the entire file. This is much faster for large files
// while still providing good uniqueness for deduplication purposes.
func hashLargeFile(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	
	// Create a hash
	h := sha256.New()
	
	// Always hash the first and last 1MB
	headSize := int64(1024 * 1024) // 1MB
	tailSize := int64(1024 * 1024) // 1MB
	
	// For files between 10MB and 100MB, sample 10% of the file
	// For files larger than 100MB, sample at most 10MB
	middleSize := int64(0)
	if size > 100*1024*1024 { // > 100MB
		middleSize = 10 * 1024 * 1024 // 10MB
	} else if size > 10*1024*1024 { // > 10MB
		middleSize = size / 10 // 10% of file
	}
	
	// Read and hash the first chunk
	buf := make([]byte, 64*1024) // 64KB buffer
	remaining := headSize
	for remaining > 0 {
		n := remaining
		if n > int64(len(buf)) {
			n = int64(len(buf))
		}
		
		read, err := io.ReadAtLeast(f, buf[:n], int(n))
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return "", err
		}
		if read == 0 {
			break
		}
		
		h.Write(buf[:read])
		remaining -= int64(read)
		
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}
	
	// If we have a middle section to sample, read samples from the middle
	if middleSize > 0 && size > headSize+tailSize {
		// Calculate the middle section boundaries
		middleStart := headSize
		middleEnd := size - tailSize
		middleRange := middleEnd - middleStart
		
		// Take samples throughout the middle section
		numSamples := 10 // Number of samples to take
		sampleSize := middleSize / int64(numSamples)
		
		for i := 0; i < numSamples; i++ {
			// Calculate position for this sample
			pos := middleStart + (middleRange * int64(i) / int64(numSamples))
			
			// Seek to the position
			_, err := f.Seek(pos, 0)
			if err != nil {
				return "", err
			}
			
			// Read a sample
			remaining := sampleSize
			for remaining > 0 {
				n := remaining
				if n > int64(len(buf)) {
					n = int64(len(buf))
				}
				
				read, err := io.ReadAtLeast(f, buf[:n], int(n))
				if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
					return "", err
				}
				if read == 0 {
					break
				}
				
				h.Write(buf[:read])
				remaining -= int64(read)
				
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					break
				}
			}
		}
	}
	
	// Read and hash the last chunk
	if size > headSize && tailSize > 0 {
		// Seek to the position for the tail section
		_, err := f.Seek(-tailSize, 2) // Seek from end
		if err != nil {
			return "", err
		}
		
		remaining := tailSize
		for remaining > 0 {
			n := remaining
			if n > int64(len(buf)) {
				n = int64(len(buf))
			}
			
			read, err := io.ReadAtLeast(f, buf[:n], int(n))
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				return "", err
			}
			if read == 0 {
				break
			}
			
			h.Write(buf[:read])
			remaining -= int64(read)
			
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
		}
	}
	
	// Include the file size in the hash calculation to ensure
	// files with the same sampled content but different sizes
	// get different hashes
	sizeBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeBuf, uint64(size))
	h.Write(sizeBuf)
	
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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
