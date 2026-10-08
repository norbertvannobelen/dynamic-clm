package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/contextmgr"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
)

func main() {
	model := flag.String("model", "Qwen3.8-27B-4bit", "model id")
	out := flag.String("out", "configs/calibration/m5max-qwen38.json", "output path")
	hardware := flag.String("hardware", "apple-m5-max", "hardware id")
	offline := flag.Bool("offline", false, "write prior without live oMLX calls")
	flag.Parse()

	if *offline {
		c := scheduler.DefaultM5MaxPrior(*model)
		if err := c.Save(*out); err != nil {
			fatal(err)
		}
		fmt.Println("wrote offline prior to", *out)
		return
	}

	client, err := omlx.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "live client failed (%v); writing offline prior\n", err)
		c := scheduler.DefaultM5MaxPrior(*model)
		_ = c.Save(*out)
		os.Exit(1)
	}
	client.MaxTokens = 16

	prefill := func(nTokens int) (time.Duration, error) {
		pad := strings.Repeat("word ", nTokens)
		res, err := client.Chat(*model, []omlx.Message{
			{Role: "user", Content: "Repeat OK. Context:\n" + pad},
		})
		return res.Elapsed, err
	}
	decode := func(kvLen, decodeTokens int) (time.Duration, error) {
		client.MaxTokens = decodeTokens
		pad := strings.Repeat("ctx ", kvLen/2)
		res, err := client.Chat(*model, []omlx.Message{
			{Role: "user", Content: "Say hi briefly.\n" + pad},
		})
		client.MaxTokens = 16
		return res.Elapsed, err
	}
	rewrite := func() (time.Duration, error) {
		start := time.Now()
		tr := &contextmgr.Tracker{}
		for i := 0; i < 20; i++ {
			id := fmt.Sprintf("span-%d", i+1)
			tr.AppendLive(id, strings.Repeat("blob ", 50))
			if i%2 == 0 {
				tr.ApplyMarkers(contextmgr.ParseMarkers(contextmgr.FormatIgnore(id)), i)
			}
		}
		rw := contextmgr.NewRewriter(nil)
		if _, err := rw.Rewrite(tr, 10); err != nil {
			return 0, err
		}
		// Include a post-rewrite re-prefill chat so C_fixed captures server overhead.
		_, err := client.Chat(*model, []omlx.Message{
			{Role: "user", Content: "Rewrite complete. Reply OK.\n" + strings.Repeat("ctx ", 256)},
		})
		return time.Since(start), err
	}

	cal, err := scheduler.RunMicrobenchmark(*hardware, *model, prefill, decode, rewrite)
	if err != nil {
		fmt.Fprintf(os.Stderr, "calibration error: %v; writing prior\n", err)
		cal = scheduler.DefaultM5MaxPrior(*model)
	}
	if err := cal.Save(*out); err != nil {
		fatal(err)
	}
	fmt.Printf("calibration: r=%.6f kd=%.8f c_fixed=%.3f rho=%.3f → %s\n", cal.R, cal.Kd, cal.CFixed, cal.Rho, *out)

	probe := client.ProbeCacheAPIs()
	fmt.Println("cache probe:")
	for p, st := range probe {
		fmt.Printf("  %s → %s\n", p, st)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
