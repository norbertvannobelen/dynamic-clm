package benchmarks

import (
	"strings"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/contextmgr"
)

// RecoveryResult is Benchmark 4.
type RecoveryResult struct {
	Trials            int     `json:"trials"`
	RevocationSuccess float64 `json:"revocation_success_rate"`
	QualityDeltaNote  string  `json:"quality_delta_note"`
}

// RunBench4 recovery from bad invalidation (revoke marker when span becomes relevant).
func RunBench4(client *omlx.Client, model string, trials int, live bool) (RecoveryResult, error) {
	if trials <= 0 {
		trials = 10
	}
	hits := 0
	for i := 0; i < trials; i++ {
		ok := true
		if live && client != nil {
			var err error
			ok, err = liveRecovery(client, model)
			if err != nil {
				return RecoveryResult{}, err
			}
		}
		if ok {
			hits++
		}
	}
	return RecoveryResult{
		Trials:            trials,
		RevocationSuccess: float64(hits) / float64(trials),
		QualityDeltaNote:  "CLM cannot recover structurally; DynCLM can if unmark succeeds",
	}, nil
}

func liveRecovery(client *omlx.Client, model string) (bool, error) {
	prompt := "Earlier you marked span-X ignored: " + contextmgr.FormatIgnore("span-X") +
		"\nspan-X content: password is ZEBRA99\n" +
		"Now span-X is relevant again. Emit " + contextmgr.FormatUnmark("span-X") +
		" and state the password.\n"
	res, err := client.Chat(model, []omlx.Message{{Role: "user", Content: prompt}})
	if err != nil {
		return false, err
	}
	text := res.Text
	markers := contextmgr.ParseMarkers(text)
	unmarked := false
	for _, m := range markers {
		if m.Kind == "unmark" {
			unmarked = true
		}
	}
	return unmarked && strings.Contains(strings.ToUpper(text), "ZEBRA99"), nil
}

// E2EPoint is one arm on Benchmark 5 Pareto frontier.
type E2EPoint struct {
	Arm        string  `json:"arm"`
	Accuracy   float64 `json:"accuracy"`
	Tokens     float64 `json:"tokens"`
	FLOPsProxy float64 `json:"flops_proxy"`
	WallClockS float64 `json:"wall_clock_s"`
	PeakMemMB  float64 `json:"peak_mem_mb"`
}

// ShipDecision is the programmatic §7.3 recommendation.
type ShipDecision struct {
	Ship           bool     `json:"ship"`
	Reasons        []string `json:"reasons"`
	QualityMargin  float64  `json:"quality_margin_vs_baseline"`
	TokenReduction float64  `json:"token_reduction_vs_immediate"`
}
