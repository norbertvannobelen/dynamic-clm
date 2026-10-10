#!/usr/bin/env bash
# Efficient 3-arm decision suite: Baseline vs DynCLM vs CLM on speed + quality.
# Does NOT run the 900-session policy grid.
set -euo pipefail
cd "$(dirname "$0")/.."

mkdir -p bin results/runs
go build -o bin/dynclm-gate ./cmd/dynclm-gate
go build -o bin/dynclm-quality ./cmd/dynclm-quality
go build -o bin/dynclm-longhorizon ./cmd/dynclm-longhorizon

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)_decision"
RUN_ROOT="results/runs/${RUN_ID}"
mkdir -p "$RUN_ROOT"
cat >"$RUN_ROOT/manifest.json" <<EOF
{
  "run_id": "${RUN_ID}",
  "started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "live": true,
  "suites": [],
  "status": "running",
  "notes": "Efficient 3-arm decision suite (short gate + quality + long-horizon A/B). Learning-900 archived separately."
}
EOF
ln -sfn "runs/${RUN_ID}" results/latest
echo "RUN_ROOT=$RUN_ROOT"

# Health check — fail fast if oMLX down
code=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8000/health || echo 000)
if [[ "$code" != "200" ]]; then
  echo "oMLX health=$code — start with: omlx start" >&2
  exit 1
fi

echo "=== 1/4 SHORT LIVE GATE (8 sessions × 40 turns × 3 arms) ==="
# Skip Bench3 here — it is a 50-call compliance curve and blocked the suite after cells finished.
./bin/dynclm-gate -config configs/gate.yaml -sessions 8 -turns 40 -skip-bench3 \
  -run-root "$RUN_ROOT" 2>&1 | tee "$RUN_ROOT/gate.log"
cp -f "$RUN_ROOT/interim-report.md" "$RUN_ROOT/decision-gate-interim.md" 2>/dev/null || true

echo "=== 2/4 LIVE GRADED QUALITY ==="
# Keep flags on one line — a broken "\\n" continuation previously aborted the suite after quality.
./bin/dynclm-quality -model Qwen3.8-27B-4bit -fresh -results "$RUN_ROOT/quality" -out "$RUN_ROOT/quality-report.md" 2>&1 | tee "$RUN_ROOT/quality.log"
cp -f "$RUN_ROOT/quality-report.md" docs/quality-report.md || true

echo "=== 3/4 LIVE LONG-HORIZON A ==="
./bin/dynclm-longhorizon -mode a -model Qwen3.8-27B-4bit -sessions 3 -turns 24 -results "$RUN_ROOT/longhorizon" -out "$RUN_ROOT/longhorizon-report.md" 2>&1 | tee "$RUN_ROOT/longhorizon-a.log"
cp -f "$RUN_ROOT/longhorizon-report.md" docs/longhorizon-report.md || true

echo "=== 4/4 LIVE LONG-HORIZON B (16k + 64k) ==="
./bin/dynclm-longhorizon -mode b -model Qwen3.8-27B-4bit -sessions 2 -turns 20 -limits 16384,65536 -results "$RUN_ROOT/longhorizon_compaction" -out "$RUN_ROOT/longhorizon-compaction-report.md" 2>&1 | tee "$RUN_ROOT/longhorizon-b.log"
cp -f "$RUN_ROOT/longhorizon-compaction-report.md" docs/longhorizon-compaction-report.md || true

# Point chart generator at this run's artifacts via copies into conventional paths used by gen_charts
mkdir -p results/quality results/longhorizon results/longhorizon_compaction_live
cp -f "$RUN_ROOT/quality/report.json" results/quality/report.json 2>/dev/null || true
cp -f "$RUN_ROOT/longhorizon/report.json" results/longhorizon/report.json 2>/dev/null || true
cp -f "$RUN_ROOT/longhorizon_compaction/matrix_report.json" results/longhorizon_compaction_live/matrix_report.json 2>/dev/null || true
cp -f "$RUN_ROOT/gate/report.json" results/gate/report.json 2>/dev/null || true

python3 scripts/gen_charts.py || true
python3 scripts/gen_decision_report.py --run-root "$RUN_ROOT" --out docs/decision-report.md
cp -f docs/decision-report.md "$RUN_ROOT/decision-report.md"

python3 - <<PY
import json, pathlib, datetime
p = pathlib.Path("$RUN_ROOT/manifest.json")
m = json.loads(p.read_text())
m["finished_at"] = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
m["status"] = "complete"
m["suites"] = ["gate", "quality", "longhorizon", "longhorizon_compaction"]
p.write_text(json.dumps(m, indent=2) + "\n")
print("DONE decision suite", m["run_id"])
PY

echo "decision report → docs/decision-report.md"
echo "run archive → $RUN_ROOT"
