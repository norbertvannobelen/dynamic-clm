package benchmarks

import (
	"fmt"
	"strings"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/contextmgr"
)

// RetentionPoint is Benchmark 1 output.
type RetentionPoint struct {
	ConstraintPos string  `json:"constraint_pos"`
	Strength      string  `json:"strength"`
	TurnIndex     int     `json:"turn_index"`
	Retention     float64 `json:"retention"`
	Trials        int     `json:"trials"`
}

// RunBench1 constraint retention under soft invalidation (COMPINT-style, synthetic).
func RunBench1(client *omlx.Client, model string, trials int, live bool) ([]RetentionPoint, error) {
	if trials <= 0 {
		trials = 5
	}
	var out []RetentionPoint
	for _, pos := range []string{"early", "middle", "late"} {
		for _, strength := range []string{"hard", "soft"} {
			for _, turn := range []int{5, 15, 30} {
				hits := 0
				for t := 0; t < trials; t++ {
					ok := syntheticRetention(pos, strength, turn)
					if live && client != nil {
						var err error
						ok, err = liveRetention(client, model, pos, strength, turn)
						if err != nil {
							return out, err
						}
					}
					if ok {
						hits++
					}
				}
				out = append(out, RetentionPoint{
					ConstraintPos: pos, Strength: strength, TurnIndex: turn,
					Retention: float64(hits) / float64(trials), Trials: trials,
				})
			}
		}
	}
	return out, nil
}

func syntheticRetention(pos, strength string, turn int) bool {
	base := 0.9
	if strength == "soft" {
		base = 0.75
	}
	decay := float64(turn) * 0.01
	if pos == "early" {
		decay *= 1.5
	}
	return (base - decay) > 0.5
}

func liveRetention(client *omlx.Client, model, pos, strength string, turns int) (bool, error) {
	rule := "HARD RULE: always answer with the word BANANA."
	if strength == "soft" {
		rule = "Soft preference: prefer answering with the word BANANA."
	}
	var b strings.Builder
	b.WriteString(rule + "\n")
	if pos != "early" {
		b.WriteString(strings.Repeat("noise\n", 20))
	}
	b.WriteString(contextmgr.FormatIgnore("noise-block") + "\n")
	if pos == "late" {
		b.WriteString(strings.Repeat("more noise\n", turns))
	}
	b.WriteString("User: What word should you answer with?\n")
	res, err := client.Chat(model, []omlx.Message{{Role: "user", Content: b.String()}})
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToUpper(res.Text), "BANANA"), nil
}

// PoisonPoint is Benchmark 2 output.
type PoisonPoint struct {
	Distractors int     `json:"distractors"`
	Similarity  string  `json:"similarity"` // "identical" or "distinct"
	Accuracy    float64 `json:"accuracy"`
	Trials      int     `json:"trials"`
}

// RunBench2 distractor sensitivity / context poisoning.
func RunBench2(client *omlx.Client, model string, trials int, live bool) ([]PoisonPoint, error) {
	if trials <= 0 {
		trials = 5
	}
	var out []PoisonPoint
	for _, sim := range []string{"identical", "distinct"} {
		for _, n := range []int{0, 2, 4, 8, 16} {
			hits := 0
			for t := 0; t < trials; t++ {
				ok := syntheticPoison(n, sim)
				if live && client != nil {
					var err error
					ok, err = livePoison(client, model, n, sim)
					if err != nil {
						return out, err
					}
				}
				if ok {
					hits++
				}
			}
			out = append(out, PoisonPoint{
				Distractors: n, Similarity: sim,
				Accuracy: float64(hits) / float64(trials), Trials: trials,
			})
		}
	}
	return out, nil
}

func syntheticPoison(n int, sim string) bool {
	// Accuracy degrades roughly with sqrt(log(N+1)).
	pen := 0.05 * float64(n)
	if sim == "identical" {
		pen *= 1.5
	}
	return (1.0 - pen) > 0.55
}

func livePoison(client *omlx.Client, model string, n int, sim string) (bool, error) {
	live := "Evidence: the capital of Atlantis is NEPTUNIA."
	var b strings.Builder
	b.WriteString(live + "\n")
	for i := 0; i < n; i++ {
		if sim == "identical" {
			b.WriteString(fmt.Sprintf("Evidence: the capital of Atlantis is FAKE_%d.\n", i))
		} else {
			b.WriteString(contextmgr.FormatIgnore(fmt.Sprintf("fake-%d", i)) + "\n")
			b.WriteString(fmt.Sprintf("[ignored blob %d]\n", i))
		}
	}
	b.WriteString("User: What is the capital of Atlantis?\n")
	res, err := client.Chat(model, []omlx.Message{{Role: "user", Content: b.String()}})
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToUpper(res.Text), "NEPTUNIA"), nil
}
