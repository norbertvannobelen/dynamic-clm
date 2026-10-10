#!/usr/bin/env bash
# Resume a paused live pipeline from results/runs/<id>/ (or results/latest).
# Completed cells are skipped via CellDone; in-progress session at pause is redone.
set -euo pipefail
cd "$(dirname "$0")/.."

RUN_ROOT="${1:-}"
if [[ -z "$RUN_ROOT" ]]; then
  if [[ -L results/latest ]]; then
    RUN_ROOT="results/$(readlink results/latest)"
  elif [[ -f results/latest.run_id ]]; then
    RUN_ROOT="results/runs/$(tr -d '[:space:]' < results/latest.run_id)"
  else
    echo "usage: $0 results/runs/<run_id>" >&2
    exit 1
  fi
fi
# normalize
if [[ "$RUN_ROOT" != results/* ]]; then
  RUN_ROOT="results/runs/$(basename "$RUN_ROOT")"
fi
if [[ ! -d "$RUN_ROOT" ]]; then
  echo "missing run root: $RUN_ROOT" >&2
  exit 1
fi

mkdir -p bin
go build -o bin/dynclm-gate ./cmd/dynclm-gate
go build -o bin/dynclm-eval ./cmd/dynclm-eval
go build -o bin/dynclm-quality ./cmd/dynclm-quality
go build -o bin/dynclm-longhorizon ./cmd/dynclm-longhorizon

ln -sfn "runs/$(basename "$RUN_ROOT")" results/latest

# Clear paused status → running
python3 - <<PY
import json, pathlib, datetime
p = pathlib.Path("$RUN_ROOT/manifest.json")
m = json.loads(p.read_text()) if p.exists() else {}
m["run_id"] = m.get("run_id") or pathlib.Path("$RUN_ROOT").name
m["status"] = "running"
m["notes"] = (m.get("notes") or "") + "; resumed " + datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
m.pop("finished_at", None)
m.pop("paused_at", None)
p.write_text(json.dumps(m, indent=2) + "\n")
print("resuming", m["run_id"])
PY

echo "RUN_ROOT=$RUN_ROOT"
echo "=== RESUME: ensure gate PASSED ==="
if [[ ! -f results/gate/PASSED ]]; then
  if [[ -f "$RUN_ROOT/gate/PASSED" ]]; then
    mkdir -p results/gate
    cp -f "$RUN_ROOT/gate/PASSED" results/gate/PASSED
    [[ -f "$RUN_ROOT/gate/report.json" ]] && cp -f "$RUN_ROOT/gate/report.json" results/gate/report.json
  else
    echo "No gate PASSED — re-running live gate into this run root"
    ./bin/dynclm-gate -config configs/gate.yaml -run-root "$RUN_ROOT" 2>&1 | tee -a "$RUN_ROOT/gate.log"
  fi
fi
test -f results/gate/PASSED

echo "=== RESUME: LIVE LEARNING (skips completed cells) ==="
./bin/dynclm-eval -mode learning -config configs/learning.yaml -run-root "$RUN_ROOT" 2>&1 | tee -a "$RUN_ROOT/learning.log"

echo "=== RESUME: LIVE QUALITY ==="
./bin/dynclm-quality -model Qwen3.8-27B-4bit -fresh -results "$RUN_ROOT/quality" -out "$RUN_ROOT/quality-report.md" 2>&1 | tee -a "$RUN_ROOT/quality.log"
cp -f "$RUN_ROOT/quality-report.md" docs/quality-report.md || true

echo "=== RESUME: LIVE LONG-HORIZON A ==="
./bin/dynclm-longhorizon -mode a -model Qwen3.8-27B-4bit -sessions 3 -turns 24 \
  -results "$RUN_ROOT/longhorizon" -out "$RUN_ROOT/longhorizon-report.md" 2>&1 | tee -a "$RUN_ROOT/longhorizon-a.log"
cp -f "$RUN_ROOT/longhorizon-report.md" docs/longhorizon-report.md || true

echo "=== RESUME: LIVE LONG-HORIZON B ==="
./bin/dynclm-longhorizon -mode b -model Qwen3.8-27B-4bit -sessions 2 -turns 20 \
  -results "$RUN_ROOT/longhorizon_compaction" -out "$RUN_ROOT/longhorizon-compaction-report.md" 2>&1 | tee -a "$RUN_ROOT/longhorizon-b.log"
cp -f "$RUN_ROOT/longhorizon-compaction-report.md" docs/longhorizon-compaction-report.md || true

python3 scripts/gen_charts.py || true
python3 scripts/gen_analysis_report.py --runs-root results/runs --out docs/analysis-report.md

python3 - <<PY
import json, pathlib, datetime
p = pathlib.Path("$RUN_ROOT/manifest.json")
m = json.loads(p.read_text())
m["finished_at"] = datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
m["status"] = "complete"
suites = set(m.get("suites") or [])
suites.update(["gate","learning","quality","longhorizon","longhorizon_compaction"])
m["suites"] = sorted(suites)
p.write_text(json.dumps(m, indent=2) + "\n")
print("finished", m["run_id"])
PY

echo "DONE resume run=$(basename "$RUN_ROOT") analysis → docs/analysis-report.md"
