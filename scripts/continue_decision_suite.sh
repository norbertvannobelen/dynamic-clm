#!/usr/bin/env bash
# Continue a decision-suite run after gate+quality (skips completed LH cells).
set -euo pipefail
cd "$(dirname "$0")/.."

RUN_ROOT="${1:-results/runs/20261010T145959Z_decision}"
if [[ ! -d "$RUN_ROOT" ]]; then
  echo "missing $RUN_ROOT" >&2
  exit 1
fi

mkdir -p bin
go build -o bin/dynclm-longhorizon ./cmd/dynclm-longhorizon
ln -sfn "runs/$(basename "$RUN_ROOT")" results/latest

code=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8000/health || echo 000)
if [[ "$code" != "200" ]]; then
  echo "oMLX health=$code - run: omlx start" >&2
  exit 1
fi

python3 - <<PY
import json
from pathlib import Path
p = Path("$RUN_ROOT/manifest.json")
m = json.loads(p.read_text()) if p.exists() else {}
m["status"] = "running"
m.pop("finished_at", None)
m["notes"] = (m.get("notes") or "") + "; continue LH A/B"
suites = set(m.get("suites") or [])
suites.update(["gate", "quality"])
m["suites"] = sorted(suites)
p.write_text(json.dumps(m, indent=2) + "\n")
print("continuing", m.get("run_id"), "status=running")
PY

echo "=== 3/4 LIVE LONG-HORIZON A ==="
./bin/dynclm-longhorizon -mode a -model Qwen3.8-27B-4bit -sessions 3 -turns 24 \
  -results "$RUN_ROOT/longhorizon" -out "$RUN_ROOT/longhorizon-report.md" \
  2>&1 | tee "$RUN_ROOT/longhorizon-a.log"
cp -f "$RUN_ROOT/longhorizon-report.md" docs/longhorizon-report.md || true

echo "=== 4/4 LIVE LONG-HORIZON B (16k + 64k) ==="
./bin/dynclm-longhorizon -mode b -model Qwen3.8-27B-4bit -sessions 2 -turns 20 \
  -limits 16384,65536 \
  -results "$RUN_ROOT/longhorizon_compaction" -out "$RUN_ROOT/longhorizon-compaction-report.md" \
  2>&1 | tee "$RUN_ROOT/longhorizon-b.log"
cp -f "$RUN_ROOT/longhorizon-compaction-report.md" docs/longhorizon-compaction-report.md || true

mkdir -p results/quality results/longhorizon/cells results/longhorizon_compaction_live results/gate
cp -f "$RUN_ROOT/quality/report.json" results/quality/report.json 2>/dev/null || true
cp -f "$RUN_ROOT/longhorizon/report.json" results/longhorizon/report.json 2>/dev/null || true
if [[ -d "$RUN_ROOT/longhorizon/cells" ]]; then
  rm -rf results/longhorizon/cells
  cp -R "$RUN_ROOT/longhorizon/cells" results/longhorizon/cells
fi
cp -f "$RUN_ROOT/longhorizon_compaction/matrix_report.json" results/longhorizon_compaction_live/matrix_report.json 2>/dev/null || true
cp -f "$RUN_ROOT/gate/report.json" results/gate/report.json 2>/dev/null || true

python3 scripts/gen_charts.py || true
python3 scripts/gen_decision_report.py --run-root "$RUN_ROOT" --out docs/decision-report.md
cp -f docs/decision-report.md "$RUN_ROOT/decision-report.md"

python3 - <<PY
import json
from pathlib import Path
from datetime import datetime, timezone
p = Path("$RUN_ROOT/manifest.json")
m = json.loads(p.read_text())
m["finished_at"] = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
m["status"] = "complete"
m["suites"] = ["gate", "quality", "longhorizon", "longhorizon_compaction"]
p.write_text(json.dumps(m, indent=2) + "\n")
print("DONE decision suite", m["run_id"])
PY

echo "decision report → docs/decision-report.md"
