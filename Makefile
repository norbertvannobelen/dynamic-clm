MODULE := github.com/norbertvannobelen/dynamic-clm
BIN := bin

.PHONY: all build test vet gate calibrate baseline learning quality report clean longhorizon longhorizon-b charts

quality: build
	./$(BIN)/dynclm-quality -model Qwen3.8-27B-4bit -out docs/quality-report.md

longhorizon: build
	./$(BIN)/dynclm-longhorizon -model Qwen3.8-27B-4bit -out docs/longhorizon-report.md

longhorizon-b: build
	./$(BIN)/dynclm-longhorizon -mode b -model Qwen3.8-27B-4bit -sessions 1 -turns 20 \
		-out docs/longhorizon-compaction-report.md -results results/longhorizon_compaction

charts:
	python3 scripts/gen_charts.py

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
