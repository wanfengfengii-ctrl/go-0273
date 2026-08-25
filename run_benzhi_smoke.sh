#!/usr/bin/env bash
# 冒烟测试：编译并启动服务，探测健康检查，并通过真实公开 API 完成一次
# 目录写入、任务创建与锁定，验证稳定业务码后清理所有进程与临时文件。
# 不依赖外部网络，不以 go test 充当冒烟测试。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${ROOT}"

TMPDIR_SMOKE="$(mktemp -d)"
DB_PATH="${TMPDIR_SMOKE}/benzhi.db"
BIN="${TMPDIR_SMOKE}/benzhi-server"
PORT="${BENZHI_SMOKE_PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"
SERVER_PID=""

cleanup() {
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${TMPDIR_SMOKE}"
}
trap cleanup EXIT

echo "==> building server"
CGO_ENABLED=0 go build -o "${BIN}" .

echo "==> starting server on :${PORT}"
BENZHI_DB_PATH="${DB_PATH}" BENZHI_ADDR=":${PORT}" "${BIN}" &
SERVER_PID=$!

# 等待就绪，最多约 5 秒。
ready=""
for _ in $(seq 1 50); do
  ready="$(curl -s "${BASE}/healthz" || true)"
  if [[ "${ready}" == *'"status":"ok"'* ]]; then
    break
  fi
  sleep 0.1
done
if [[ "${ready}" != *'"status":"ok"'* ]]; then
  echo "error: server did not become healthy; last response: ${ready}" >&2
  exit 1
fi
echo "==> healthz ok"

readyz="$(curl -s "${BASE}/readyz")"
if [[ "${readyz}" != *'"status":"ready"'* ]]; then
  echo "error: readyz failed: ${readyz}" >&2
  exit 1
fi
echo "==> readyz ok"

post_json() {
  local path="$1" body="$2" op="$3"
  curl -s -X POST "${BASE}${path}" \
    -H 'Content-Type: application/json' \
    -H "X-Operation-ID: ${op}" \
    -d "${body}"
}

echo "==> seeding catalog"
post_json /api/v1/catalog/mothers \
  '{"id":"M1","strain":"s","lineage_id":"L1","enabled":true,"version":1}' op-mother >/dev/null
post_json /api/v1/catalog/media \
  '{"formula_id":"F1","revision":1,"summary":"V1","effective_at":1}' op-medium >/dev/null
post_json /api/v1/catalog/rules \
  '{"id":"R1","vitrification_max_pct":30,"browning_max_pct":20,"root_viability_min":50,"virus_ct_max":350,"endophyte_cfu_max":1000,"fixed_scale":2}' op-rule >/dev/null
for pid in P1 P2 R1 R2; do
  post_json /api/v1/catalog/personnel \
    "{\"id\":\"${pid}\",\"qualifications\":[\"subculture\",\"review\"],\"enabled\":true}" "op-${pid}" >/dev/null
done

echo "==> creating and locking a task"
create_body='{"task_id":"SMOKE1","mother_plant_id":"M1","lineage_id":"L1","medium_formula_id":"F1","medium_summary":"V1","subculture_batch":"B-SMOKE","bottles":[{"seal":"S1","position":"P1","locked_seedlings":10}],"blind_codes":["BC1"],"root_points":["RP1"],"vitrification_positions":["VP1"],"rt_pcr_wells":["W1"],"endophyte_wells":["EW1"],"rack_shelf":"RS1","light_window":"LW1","acclimation_window":"AW1","thresholds":{"vitrification_max_pct":30,"browning_max_pct":20,"root_viability_min":50,"virus_ct_max":350,"endophyte_cfu_max":1000},"allowed_personnel":["P1","P2","R1","R2"],"task_generation":1}'
create_resp="$(post_json /api/v1/tasks "${create_body}" op-create)"
if [[ "${create_resp}" != *'"code":"OK"'* ]]; then
  echo "error: create task failed: ${create_resp}" >&2
  exit 1
fi
echo "==> create ok"

lock_resp="$(post_json /api/v1/tasks/SMOKE1/lock '{}' op-lock)"
if [[ "${lock_resp}" != *'"code":"OK"'* ]]; then
  echo "error: lock task failed: ${lock_resp}" >&2
  exit 1
fi
echo "==> lock ok"

task_resp="$(curl -s "${BASE}/api/v1/tasks/SMOKE1")"
if [[ "${task_resp}" != *'"state":"pending_subculture"'* ]]; then
  echo "error: unexpected task state: ${task_resp}" >&2
  exit 1
fi
echo "==> task state ok"

echo "SMOKE PASSED"
