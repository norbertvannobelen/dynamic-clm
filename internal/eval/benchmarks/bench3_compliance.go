package benchmarks

import (
	"fmt"
	"os"
	"strings"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/contextmgr"
)

// CompliancePoint is one measurement on the instruction-decay curve.
type CompliancePoint struct {
	MarkerPosition string  `json:"marker_position"` // "tail" or "middle"
	TurnsSince     int     `json:"turns_since_marker"`
	Compliance     float64 `json:"compliance"` // 0..1
	Trials         int     `json:"trials"`
}

// RunBench3 measures ignore-marker compliance vs turns-since-marker.
func RunBench3(client *omlx.Client, model string, trials int, live bool) ([]CompliancePoint, error) {
	if trials <= 0 {
		trials = 5
	}
	delays := []int{1, 3, 5, 10, 20}
	positions := []string{"tail", "middle"}
	var out []CompliancePoint

	total := len(positions) * len(delays) * trials
	n := 0
	for _, pos := range positions {
		for _, delay := range delays {
			hits := 0
			for t := 0; t < trials; t++ {
				n++
				if live && (n == 1 || n%5 == 0 || n == total) {
					fmt.Fprintf(os.Stderr, "bench3 progress %d/%d pos=%s delay=%d trial=%d\n", n, total, pos, delay, t+1)
				}
				ok, err := oneComplianceTrial(client, model, pos, delay, live)
				if err != nil {
					return out, err
				}
				if ok {
					hits++
				}
			}
			out = append(out, CompliancePoint{
				MarkerPosition: pos,
				TurnsSince:     delay,
				Compliance:     float64(hits) / float64(trials),
				Trials:         trials,
			})
		}
	}
	return out, nil
}

func oneComplianceTrial(client *omlx.Client, model, pos string, delay int, live bool) (bool, error) {
	secret := "PURPLE_ ELEPHANT_99"
	marker := contextmgr.FormatIgnore("secret-fact")
	fact := "The secret code is " + secret + "."

	var history strings.Builder
	history.WriteString("System: Follow ignore markers of the form <<<DYNCLM_IGNORE ...>>>.\n")
	if pos == "middle" {
		history.WriteString(fact + "\n")
		history.WriteString(marker + "\n")
		for i := 0; i < delay; i++ {
			history.WriteString(fmt.Sprintf("Filler turn %d: lorem ipsum coding noise.\n", i))
		}
	} else {
		for i := 0; i < delay; i++ {
			history.WriteString(fmt.Sprintf("Filler turn %d: lorem ipsum coding noise.\n", i))
		}
		history.WriteString(fact + "\n")
		history.WriteString(marker + "\n")
	}
	prompt := history.String() + "\nUser: What is the secret code? If ignored, say IGNORED.\n"

	if !live || client == nil {
		// Synthetic: tail markers always comply; middle decays with delay.
		if pos == "tail" {
			return true, nil
		}
		return delay <= 5, nil
	}

	res, err := client.Chat(model, []omlx.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return false, err
	}
	text := strings.ToUpper(res.Text)
	if strings.Contains(text, "IGNORED") {
		return true, nil
	}
	if strings.Contains(text, secret) {
		return false, nil
	}
	// Ambiguous — count as compliant if secret not echoed.
	return true, nil
}
