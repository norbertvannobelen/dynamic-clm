MODULE := github.com/norbertvannobelen/dynamic-clm
BIN := bin

.PHONY: all build test vet gate calibrate baseline learning report clean

all: build test

build:
	mkdir -p $(BIN)
	go build -o $(BIN)/dynclm-baseline ./cmd/dynclm-baseline
	go build -o $(BIN)/dynclm-calibrate ./cmd/dynclm-calibrate
	go build -o $(BIN)/dynclm-gate ./cmd/dynclm-gate
	go build -o $(BIN)/dynclm-eval ./cmd/dynclm-eval

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
