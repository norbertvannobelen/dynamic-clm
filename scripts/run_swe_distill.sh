#!/usr/bin/env bash
# Live SWE-Distill suite: Baseline+compact vs DynCLM vs CLM (~2h/arm target).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p bin results/runs docs/figures
go build -o bin/dynclm-swedistill ./cmd/dynclm-swedistill

MODEL="${MODEL:-Qwen3.8-27B-4bit}"
TURNS="${TURNS:-28}"
MAX_CTX="${MAX_CTX:-16384}"

code=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8000/health || echo 000)
if [[ "$code" != "200" ]]; then
  echo "oMLX health=$code - start with: omlx start" >&2
  exit 1
fi

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)_swe_distill"
RUN_ROOT="results/runs/${RUN_ID}"
mkdir -p "$RUN_ROOT"
cat >"$RUN_ROOT/manifest.json" <<EOF
{
  "run_id": "${RUN_ID}",
  "started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "live": true,
  "suites": ["swe_distill"],
  "status": "running",
  "notes": "SWE-Distill Baseline+compact vs DynCLM vs CLM under shared max_context=${MAX_CTX}"
}
EOF
ln -sfn "runs/${RUN_ID}" results/latest
echo "RUN_ROOT=$RUN_ROOT"

echo "=== SWE-DISTILL live model=$MODEL turns=$TURNS max_context=$MAX_CTX ==="
./bin/dynclm-swedistill \
  -model "$MODEL" \
  -turns "$TURNS" \
  -max-context "$MAX_CTX" \
  -run-root "$RUN_ROOT" \
  -out "$RUN_ROOT/swe-distill-report.md" \
  2>&1 | tee "$RUN_ROOT/swe-distill.log"

REPORT_JSON="$RUN_ROOT/swe_distill/report.json"
if [[ -f "$REPORT_JSON" ]]; then
  python3 scripts/gen_swe_distill_report.py --report "$REPORT_JSON" --out docs/swe-distill-report.md
  cp -f docs/swe-distill-report.md "$RUN_ROOT/swe-distill-report.md"
  # mark complete
  python3 - <<PY
import json, datetime
from pathlib import Path
p = Path("$RUN_ROOT/manifest.json")
m = json.loads(p.read_text())
m["status"] = "complete"
m["finished_at"] = datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
p.write_text(json.dumps(m, indent=2) + "\n")
PY
  echo "extensive report → docs/swe-distill-report.md"
fi

echo "DONE swe-distill $RUN_ID"
