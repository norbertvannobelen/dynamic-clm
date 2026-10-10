package swedistill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Fixture is one SWE-Bench-inspired coding task.
type Fixture struct {
	InstanceID     string   `json:"instance_id"`
	Repo           string   `json:"repo"`
	Issue          string   `json:"issue"`
	CorrectSnippet string   `json:"correct_snippet"`
	WrongSnippets  []string `json:"wrong_snippets"`
	ExpectedAnswer string   `json:"expected_answer"`
	WrongAnswer    string   `json:"wrong_answer"` // recency-trap fragment shown in obsolete SPECs
	ProbeQuestion  string   `json:"probe_question"`
	Attribution    string   `json:"attribution"`
}

// LoadFixtures reads all *.json fixtures from dir (skips README).
func LoadFixtures(dir string) ([]Fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Fixture
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var f Fixture
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if f.InstanceID == "" || f.ExpectedAnswer == "" {
			return nil, fmt.Errorf("%s: missing instance_id or expected_answer", e.Name())
		}
		if f.WrongAnswer == "" && len(f.WrongSnippets) > 0 {
			// Fallback: last non-empty line of first wrong snippet (best-effort).
			f.WrongAnswer = "outdated_fix"
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceID < out[j].InstanceID })
	if len(out) == 0 {
		return nil, fmt.Errorf("no fixtures in %s", dir)
	}
	return out, nil
}
