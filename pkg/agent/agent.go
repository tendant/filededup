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
	"strings"
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
	RootDir   string
	ServerURL string
	MachineID string
	BatchSize int
}

func New(root, server, machineID string, batch int) *Agent {
	return &Agent{
		RootDir:   root,
		ServerURL: strings.TrimRight(server, "/"),
		MachineID: machineID,
		BatchSize: batch,
	}
}

func (a *Agent) Run() error {
	var batch []FileRecord

	err := filepath.Walk(a.RootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		hash, err := hashFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(a.RootDir, filepath.Dir(path))
		filename := filepath.Base(path)

		batch = append(batch, FileRecord{
			MachineID: a.MachineID,
			Path:      relPath,
			Filename:  filename,
			Size:      info.Size(),
			MTime:     info.ModTime(),
			Hash:      hash,
		})

		if len(batch) >= a.BatchSize {
			if err := a.sendBatch(batch); err != nil {
				return err
			}
			batch = nil
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("walk error: %w", err)
	}
	if len(batch) > 0 {
		return a.sendBatch(batch)
	}
	return nil
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
