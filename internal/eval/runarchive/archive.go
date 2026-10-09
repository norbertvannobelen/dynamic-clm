package runarchive

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Manifest describes one preserved evaluation run.
type Manifest struct {
	RunID       string            `json:"run_id"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	Live        bool              `json:"live"`
	Suites      []string          `json:"suites"`
	GitSHA      string            `json:"git_sha,omitempty"`
	Notes       string            `json:"notes,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
	Status      string            `json:"status"` // running|complete|failed
}

// Run is a results/runs/<id>/ directory that holds all suite artifacts for one execution.
type Run struct {
	ID   string
	Root string
	Man  Manifest
}

// Open attaches to an existing run directory (loads manifest if present).
func Open(root string) (*Run, error) {
	r := &Run{ID: filepath.Base(root), Root: root}
	b, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err == nil {
		_ = json.Unmarshal(b, &r.Man)
	}
	if r.Man.RunID == "" {
		r.Man.RunID = r.ID
	}
	if r.Man.Status == "" {
		r.Man.Status = "running"
	}
	if r.Man.Extra == nil {
		r.Man.Extra = map[string]string{}
	}
	return r, nil
}

// Begin creates results/runs/<timestamp>/ and points results/latest at it.
func Begin(resultsRoot string, live bool, notes string) (*Run, error) {
	if resultsRoot == "" {
		resultsRoot = "results"
	}
	id := time.Now().UTC().Format("20060102T150405Z")
	root := filepath.Join(resultsRoot, "runs", id)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	r := &Run{
		ID:   id,
		Root: root,
		Man: Manifest{
			RunID:     id,
			StartedAt: time.Now().UTC(),
			Live:      live,
			Suites:    nil,
			GitSHA:    gitSHA(),
			Notes:     notes,
			Status:    "running",
			Extra:     map[string]string{},
		},
	}
	if err := r.writeManifest(); err != nil {
		return nil, err
	}
	latest := filepath.Join(resultsRoot, "latest")
	_ = os.Remove(latest)
	// Prefer symlink; fall back to a pointer file.
	if err := os.Symlink(filepath.Join("runs", id), latest); err != nil {
		_ = os.WriteFile(latest+".run_id", []byte(id+"\n"), 0o644)
	}
	return r, nil
}

// SuiteDir returns (and creates) a subdirectory for a named suite.
func (r *Run) SuiteDir(name string) (string, error) {
	p := filepath.Join(r.Root, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	found := false
	for _, s := range r.Man.Suites {
		if s == name {
			found = true
			break
		}
	}
	if !found {
		r.Man.Suites = append(r.Man.Suites, name)
		_ = r.writeManifest()
	}
	return p, nil
}

// Finish marks the run complete or failed.
func (r *Run) Finish(ok bool, notes string) error {
	now := time.Now().UTC()
	r.Man.FinishedAt = &now
	if ok {
		r.Man.Status = "complete"
	} else {
		r.Man.Status = "failed"
	}
	if notes != "" {
		if r.Man.Notes != "" {
			r.Man.Notes = r.Man.Notes + "; " + notes
		} else {
			r.Man.Notes = notes
		}
	}
	return r.writeManifest()
}

func (r *Run) writeManifest() error {
	return writeJSON(filepath.Join(r.Root, "manifest.json"), r.Man)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func gitSHA() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return string(bytesTrim(out))
}

func bytesTrim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// ListRuns returns run directories under results/runs, newest first.
func ListRuns(resultsRoot string) ([]string, error) {
	if resultsRoot == "" {
		resultsRoot = "results"
	}
	dir := filepath.Join(resultsRoot, "runs")
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for i := len(ents) - 1; i >= 0; i-- {
		if ents[i].IsDir() {
			out = append(out, filepath.Join(dir, ents[i].Name()))
		}
	}
	return out, nil
}

// RefuseSynthetic errors unless allowSynthetic is set when live is false.
func RefuseSynthetic(live, allowSynthetic bool) error {
	if live {
		return nil
	}
	if allowSynthetic {
		return nil
	}
	return fmt.Errorf("live evaluation required: refusing synthetic/offline run (pass --allow-synthetic to override)")
}
