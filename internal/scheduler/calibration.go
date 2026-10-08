package scheduler

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

// CalibrationResult is the per-device signature.
type CalibrationResult struct {
	HardwareID string    `json:"hardware_id"`
	ModelID    string    `json:"model_id"`
	R          float64   `json:"r_ms_per_token"`
	Kd         float64   `json:"kd_ms_per_token_per_turn"`
	CFixed     float64   `json:"c_fixed_ms"`
	Rho        float64   `json:"rho"`
	MeasuredAt time.Time `json:"measured_at"`
	Notes      string    `json:"notes,omitempty"`
}

// ToDeviceCosts converts calibration to runtime costs.
func (c CalibrationResult) ToDeviceCosts() DeviceCosts {
	return DeviceCosts{R: c.R, Kd: c.Kd, CFixed: c.CFixed}
}

// Save writes calibration JSON.
func (c CalibrationResult) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// LoadCalibration reads a calibration file.
func LoadCalibration(path string) (CalibrationResult, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return CalibrationResult{}, err
	}
	var c CalibrationResult
	if err := json.Unmarshal(b, &c); err != nil {
		return CalibrationResult{}, err
	}
	return c, nil
}

// PrefillTimer measures wall-clock for a timed operation over n tokens.
type PrefillTimer func(nTokens int) (elapsed time.Duration, err error)

// DecodeTimer measures decode cost at a given KV length for n decode tokens.
type DecodeTimer func(kvLen, decodeTokens int) (elapsed time.Duration, err error)

// RewriteTimer measures one rewrite end-to-end.
type RewriteTimer func() (elapsed time.Duration, err error)

// RunMicrobenchmark estimates r, k_d, C_fixed from timers.
func RunMicrobenchmark(hardware, model string, prefill PrefillTimer, decode DecodeTimer, rewrite RewriteTimer) (CalibrationResult, error) {
	const prefillN = 512
	const decodeN = 32
	const kvLen = 2048

	pDur, err := prefill(prefillN)
	if err != nil {
		return CalibrationResult{}, fmt.Errorf("prefill: %w", err)
	}
	dDur, err := decode(kvLen, decodeN)
	if err != nil {
		return CalibrationResult{}, fmt.Errorf("decode: %w", err)
	}
	rDur, err := rewrite()
	if err != nil {
		return CalibrationResult{}, fmt.Errorf("rewrite: %w", err)
	}

	r := float64(pDur.Milliseconds()) / float64(prefillN)
	// Per-token decode attention cost attributed across the KV length.
	// Use a floor so Apple Silicon priors do not drive N→0 spuriously.
	kd := float64(dDur.Milliseconds()) / float64(decodeN*kvLen)
	if kd < 1e-4 {
		kd = 1e-4
	}
	_ = math.SmallestNonzeroFloat64
	cFixed := float64(rDur.Milliseconds())
	if cFixed < 1 {
		cFixed = 1 // floor: sub-ms in-process rewrites are not representative
	}
	out := CalibrationResult{
		HardwareID: hardware,
		ModelID:    model,
		R:          r,
		Kd:         kd,
		CFixed:     cFixed,
		Rho:        cFixed / kd,
		MeasuredAt: time.Now().UTC(),
		Notes:      "microbenchmark via oMLX chat completions wall-clock",
	}
	return out, nil
}

// DefaultM5MaxPrior is a fallback prior if live calibration is unavailable.
func DefaultM5MaxPrior(model string) CalibrationResult {
	const kd = 0.0005
	const cFixed = 40.0
	return CalibrationResult{
		HardwareID: "apple-m5-max",
		ModelID:    model,
		R:          0.15,
		Kd:         kd,
		CFixed:     cFixed,
		Rho:        cFixed / kd,
		MeasuredAt: time.Now().UTC(),
		Notes:      "offline prior; replace with live calibration",
	}
}
