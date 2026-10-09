#!/usr/bin/env bash
# Full live evaluation pipeline. Refuses synthetic. Archives under results/runs/<id>/.
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p bin results/runs
go build -o bin/dynclm-gate ./cmd/dynclm-gate
go build -o bin/dynclm-eval ./cmd/dynclm-eval
go build -o bin/dynclm-quality ./cmd/dynclm-quality
go build -o bin/dynclm-longhorizon ./cmd/dynclm-longhorizon

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_ROOT="results/runs/${RUN_ID}"
mkdir -p "$RUN_ROOT"
cat >"$RUN_ROOT/manifest.json" <<EOF
{
  "run_id": "${RUN_ID}",
  "started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "live": true,
  "suites": [],
  "status": "running",
  "notes": "full live pipeline"
}
EOF
ln -sfn "runs/${RUN_ID}" results/latest
echo "RUN_ROOT=$RUN_ROOT"

echo "=== 1/5 LIVE GATE ==="
./bin/dynclm-gate -config configs/gate.yaml -run-root "$RUN_ROOT" 2>&1 | tee "$RUN_ROOT/gate.log"
test -f results/gate/PASSED

echo "=== 2/5 LIVE LEARNING (900 sessions, all models) ==="
./bin/dynclm-eval -mode learning -config configs/learning.yaml -run-root "$RUN_ROOT" 2>&1 | tee "$RUN_ROOT/learning.log"

echo "=== 3/5 LIVE QUALITY ==="
./bin/dynclm-quality -model Qwen3.8-27B-4bit -results "$RUN_ROOT/quality" -out "$RUN_ROOT/quality-report.md" 2>&1 | tee "$RUN_ROOT/quality.log"
cp -f "$RUN_ROOT/quality-report.md" docs/quality-report.md || true

echo "=== 4/5 LIVE LONG-HORIZON A ==="
./bin/dynclm-longhorizon -mode a -model Qwen3.8-27B-4bit -sessions 3 -turns 24 \
  -results "$RUN_ROOT/longhorizon" -out "$RUN_ROOT/longhorizon-report.md" 2>&1 | tee "$RUN_ROOT/longhorizon-a.log"
cp -f "$RUN_ROOT/longhorizon-report.md" docs/longhorizon-report.md || true

echo "=== 5/5 LIVE LONG-HORIZON B (compaction matrix) ==="
./bin/dynclm-longhorizon -mode b -model Qwen3.8-27B-4bit -sessions 2 -turns 20 \
  -results "$RUN_ROOT/longhorizon_compaction" -out "$RUN_ROOT/longhorizon-compaction-report.md" 2>&1 | tee "$RUN_ROOT/longhorizon-b.log"
cp -f "$RUN_ROOT/longhorizon-compaction-report.md" docs/longhorizon-compaction-report.md || true

python3 scripts/gen_charts.py || true
python3 scripts/gen_analysis_report.py --runs-root results/runs --out docs/analysis-report.md

python3 - <<PY
import json, pathlib, datetime
p = pathlib.Path("$RUN_ROOT/manifest.json")
m = json.loads(p.read_text())
m["finished_at"] = datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
m["status"] = "complete"
m["suites"] = ["gate","learning","quality","longhorizon","longhorizon_compaction"]
p.write_text(json.dumps(m, indent=2) + "\n")
print("finished", m["run_id"])
PY

echo "DONE run=$RUN_ID analysis → docs/analysis-report.md"
