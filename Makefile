MODULE := github.com/norbertvannobelen/dynamic-clm
BIN := bin

.PHONY: all build test vet gate calibrate baseline learning quality report clean longhorizon longhorizon-b charts live-full analysis

quality: build
	./$(BIN)/dynclm-quality -model Qwen3.8-27B-4bit -out docs/quality-report.md

longhorizon: build
	./$(BIN)/dynclm-longhorizon -model Qwen3.8-27B-4bit -out docs/longhorizon-report.md

longhorizon-b: build
	./$(BIN)/dynclm-longhorizon -mode b -model Qwen3.8-27B-4bit -sessions 1 -turns 20 \
		-out docs/longhorizon-compaction-report.md -results results/longhorizon_compaction

charts:
	python3 scripts/gen_charts.py

analysis:
	python3 scripts/gen_analysis_report.py --out docs/analysis-report.md

# Full live pipeline (gate → 900-session learning → quality → long-horizon A/B → analysis).
# Refuses synthetic. Archives under results/runs/<id>/. Expect many hours.
live-full: build
	bash scripts/run_live_full.sh

# Resume a paused run (default: results/latest). Skips completed learning cells.
resume-live: build
	bash scripts/resume_live_full.sh $(RUN_ROOT)

# Efficient 3-arm decision suite (short gate + quality + LH A/B). Not the 900 grid.
decision-suite: build
	bash scripts/run_decision_suite.sh

all: build test

build:
	mkdir -p $(BIN)
	go build -o $(BIN)/dynclm-baseline ./cmd/dynclm-baseline
	go build -o $(BIN)/dynclm-calibrate ./cmd/dynclm-calibrate
	go build -o $(BIN)/dynclm-gate ./cmd/dynclm-gate
	go build -o $(BIN)/dynclm-eval ./cmd/dynclm-eval
	go build -o $(BIN)/dynclm-quality ./cmd/dynclm-quality
	go build -o $(BIN)/dynclm-longhorizon ./cmd/dynclm-longhorizon

test:
	go test ./...

vet:
	go vet ./...

gate: build
	./$(BIN)/dynclm-gate -config configs/gate.yaml

calibrate: build
	./$(BIN)/dynclm-calibrate -model Qwen3.8-27B-4bit -out configs/calibration/m5max-qwen38.json

baseline: build
	./$(BIN)/dynclm-baseline -config configs/gate.yaml -arm baseline

learning: build
	./$(BIN)/dynclm-eval -mode learning -config configs/learning.yaml

report: build
	./$(BIN)/dynclm-eval -mode report

clean:
	rm -rf $(BIN)
