package tracing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Outcome is the §6.2 label tuple.
type Outcome struct {
	Success    bool    `json:"success"`
	WallClockS float64 `json:"wall_clock_s"`
	FLOPs      float64 `json:"flops"`
	PeakMemMB  float64 `json:"peak_mem_mb"`
}

// Record is one decision-point row (§6.2).
type Record struct {
	HardwareID              string    `json:"hardware_id"`
	ModelID                 string    `json:"model_id"`
	TaskType                string    `json:"task_type"`
	Arm                     string    `json:"arm"`
	SessionID               string    `json:"session_id"`
	TurnIndex               int       `json:"turn_index"`
	ContextLengthAtDecision int       `json:"context_length_at_decision"`
	DeadTokenEstimate       int       `json:"dead_token_estimate"`
	LiveTokenEstimate       int       `json:"live_token_estimate"`
	RewriteFired            bool      `json:"rewrite_fired"`
	CFixedObservedMS        float64   `json:"c_fixed_observed_ms"`
	PrefillTokensAfter      int       `json:"prefill_tokens_after_rewrite"`
	NaivePrefillTokens      int       `json:"naive_prefill_tokens"`
	DecodeAttnTokensCarried int       `json:"decode_attn_tokens_carried"`
	TurnsUntilNextRewrite   int       `json:"turns_until_next_rewrite"`
	CounterfactualRewrite   bool      `json:"counterfactual_would_rewrite"`
	CounterfactualDeferCost float64   `json:"counterfactual_defer_cost"`
	CounterfactualRewriteCost float64 `json:"counterfactual_rewrite_cost"`
	Outcome                 Outcome   `json:"outcome"`
	Timestamp               time.Time `json:"timestamp"`
}

// Recorder appends JSONL records; idempotent cell files live alongside.
type Recorder struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

// OpenRecorder creates/appends a JSONL trace file.
func OpenRecorder(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Recorder{path: path, f: f}, nil
}

// Emit writes one record.
func (r *Recorder) Emit(rec Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = r.f.Write(append(b, '\n'))
	return err
}

// Close flushes the file.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// ProgressLog appends a human-readable progress line.
func ProgressLog(resultsDir, msg string) {
	path := filepath.Join(resultsDir, "progress.log")
	_ = os.MkdirAll(resultsDir, 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().UTC().Format(time.RFC3339), msg)
}

// CellPath returns the per-cell result path for checkpoint/resume.
func CellPath(resultsDir, hardware, model, task, policy, session string) string {
	return filepath.Join(resultsDir, "cells", hardware, model, task, policy, session+".json")
}

// CellDone reports whether a cell result already exists.
func CellDone(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// WriteCell writes a cell result atomically-ish.
func WriteCell(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
