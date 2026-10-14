#!/usr/bin/env bash
# Public synthetic research only. Keep deployed services/catalogs and old reports intact.
set -euo pipefail
root="${1:?isolated snapshot root required}"
models="${2:-/srv/storage/butler/agents/local-models}"
mode="${3:-selected}"
full_scores=0
case "$mode" in
 selected) catalog=models.decisions-evaluation.json ;;
 full) catalog=models.decisions-fullscores-evaluation.json; full_scores=1 ;;
 taxonomy) catalog=models.decisions-taxonomy-v2.json ;;
 qwen) catalog=models.decisions-taxonomy-v2-qwen.json ;;
 qwen-cpuembed) catalog=models.decisions-taxonomy-v2-cpuembedding.json ;;
 qwen-workload) catalog=models.decisions-taxonomy-v2-cpuembedding.json ;;
 interpretation) catalog=models.interpretation-v3-cpuembedding.json ;;
 contracts) catalog=models.decisions-evaluation.json ;;
 *) printf '%s\n' 'mode must be selected, full, taxonomy or contracts' >&2; exit 1 ;;
esac
for path in "$root" "$models"; do
 if [[ ! "$path" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9/_-]+$ ]] || [[ "$path" == *..* ]]; then
  printf '%s\n' 'owned lab paths required' >&2; exit 1
 fi
done
src=$(realpath "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || { printf '%s\n' 'isolated source snapshot required' >&2; exit 1; }
cache="$models/.cache"
test -d "$cache/go-mod"
test -d "$models/data/models"
id="decision-replay-$(date -u +%Y%m%dT%H%M%SZ)-$$"
work="$root/.cache/$id"
report="$root/reports/$id"
mkdir -p "$work/private" "$report/output"
mkdir -p "$work/worker"
cp -a "$src/apps/local-models/." "$work/worker/"
cp "$src/internal/localmodels/decision-schema-v2.json" "$work/worker/decision-schema-v2.json"
cp "$src/internal/localmodels/interpretation-schema-v3.json" "$work/worker/interpretation-schema-v3.json"
# Prevent duplicate executions of this runner against the same source root.
exec 9>"$root/.cache/decision-replay.lock"
flock -n 9 || { printf '%s\n' 'decision replay already running' >&2; exit 1; }

builder=golang:1.27.2-bookworm
# Reuse installed base dependencies; compile the service from this snapshot.
runtime_image=$(docker inspect butler-lab-local-models-1 --format '{{.Image}}')
postgres_image=$(docker inspect butler-local-models-quality-postgres --format '{{.Image}}')
docker run --rm --network none --user "$(id -u):$(id -g)" \
 --volume "$src:/src:ro" --volume "$cache:/cache" --volume "$work:/out" --workdir /src \
 --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --entrypoint sh "$builder" -c \
 'go test -c -o /out/decisions.test ./internal/decisions && CGO_ENABLED=0 go build -o /out/local-models ./apps/local-models'
docker run --rm --network none --user 0:0 \
 --volume "$work/private:/private" --volume "$report/output:/output" \
 --entrypoint sh "$builder" -c \
 'umask 077; od -An -N48 -tx1 /dev/urandom | tr -d " \n" > /private/token; chown -R 10001:10001 /private /output; chmod 700 /private; chmod 755 /output'

containers=()
network=''
sampler=''
gpu_sampler=''
qwen_connected=false
cleanup() {
 if [[ -n "$sampler" ]]; then kill "$sampler" 2>/dev/null || true; wait "$sampler" || true; fi
 if [[ -n "$gpu_sampler" ]]; then kill "$gpu_sampler" 2>/dev/null || true; wait "$gpu_sampler" || true; fi
 for container in "${containers[@]}"; do
  docker logs "$container" > "$report/container-$container.log" 2>&1 || true
  docker rm -f "$container" >/dev/null 2>&1 || true
 done
 if $qwen_connected; then docker network disconnect "$network" butler-decision-qwen-v2 >/dev/null 2>&1 || true; fi
 if [[ -n "$network" ]]; then docker network rm "$network" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT
network=$(docker network create --internal "$id")
if [[ "$mode" == qwen* || "$mode" == interpretation ]]; then
 docker network connect --alias decision-qwen "$network" butler-decision-qwen-v2
 qwen_connected=true
fi
pg_storage=512m; pg_memory=1g
if [[ "$mode" == contracts ]]; then pg_storage=2g; pg_memory=2g; fi
pg=$(docker create --name "$id-pg" --network "$network" --network-alias decision-postgres \
 --env POSTGRES_USER=butler --env POSTGRES_DB=decisions_test --env POSTGRES_HOST_AUTH_METHOD=trust \
 --tmpfs "/var/lib/postgresql/data:rw,size=$pg_storage" --memory "$pg_memory" --memory-swap "$pg_memory" "$postgres_image")
containers+=("$pg")
docker start "$pg" >/dev/null
ready=false
for _ in $(seq 1 30); do
 if docker exec "$pg" pg_isready -U butler -d decisions_test >/dev/null; then ready=true; break; fi
 sleep 1
done
$ready || { printf '%s\n' 'isolated PostgreSQL did not become ready' >&2; exit 1; }
if [[ "$mode" == contracts ]]; then
 (cd "$src"; sha256sum internal/decisions/*.go internal/localmodels/*.go apps/local-models/*.go \
  apps/local-models/model-worker.py deploy/local-models/verify-decisions-isolated-lab.sh) > "$report/source.sha256"
 docker run --rm --user "$(id -u):$(id -g)" --network "$network" \
  --volume "$src:/src:ro" --volume "$cache:/cache" --volume "$report:/reports" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler@decision-postgres:5432/decisions_test?sslmode=disable \
  --entrypoint sh "$builder" -c \
  'go test -race -p 1 -count=1 -json ./internal/decisions ./internal/localmodels ./apps/local-models ./internal/memory/extraction ./internal/memory/catalog ./internal/memory/jobs > /reports/contracts-race.jsonl && go vet ./internal/decisions ./internal/localmodels ./apps/local-models ./internal/memory/extraction ./internal/memory/catalog ./internal/memory/jobs' \
  > "$report/contracts-validation.log" 2>&1
 printf 'Contract validation evidence: %s\n' "$report"
 exit 0
fi
service=$(docker create --name "$id-models" --network "$network" --network-alias local-models \
 --user 10001:10001 --init --read-only --memory 8g --memory-swap 8g --pids-limit 256 \
 --cap-drop ALL --security-opt no-new-privileges:true --gpus all \
 --tmpfs /tmp:rw,nosuid,nodev,size=256m,uid=10001,gid=10001 \
 --volume "$models/data:/data:ro" --volume "$work/private:/private:ro" \
 --volume "$work/local-models:/runner:ro" --volume "$work/worker:/worker:ro" \
 --volume "$src/deploy/local-models/$catalog:/catalog.json:ro" \
 --env BUTLER_LOCAL_MODELS_TOKEN_FILE=/private/token --env BUTLER_LOCAL_MODELS_CATALOG=/catalog.json \
 --env BUTLER_LOCAL_MODELS_WORKER=/worker/model-worker.py \
 --env BUTLER_LOCAL_MODELS_RAM_BUDGET_MIB=6144 --env BUTLER_LOCAL_MODELS_RAM_RESERVE_MIB=512 \
 --env BUTLER_LOCAL_MODELS_BUDGET_MIB=6144 --env BUTLER_LOCAL_MODELS_RESERVE_MIB=512 \
 --env BUTLER_LOCAL_MODELS_IDLE_SECONDS=30 --env NVIDIA_DRIVER_CAPABILITIES=compute,utility \
 --entrypoint /runner "$runtime_image")
containers+=("$service")
docker start "$service" >/dev/null
ready=false
for _ in $(seq 1 30); do
 if docker exec "$service" python -c 'import urllib.request; urllib.request.urlopen("http://127.0.0.1:8080/health",timeout=2)' 2>/dev/null; then ready=true; break; fi
 sleep 1
done
$ready || { printf '%s\n' 'isolated model service did not become ready' >&2; exit 1; }

(cd "$src"; sha256sum internal/decisions/*.go internal/localmodels/ram.go internal/localmodels/process.go internal/localmodels/classification.go apps/local-models/model-worker.py \
 "deploy/local-models/$catalog" \
 deploy/local-models/russian-decisions.json deploy/local-models/models.decisions-evaluation.json \
 deploy/local-models/requirements.lock.txt deploy/local-models/verify-decisions-isolated-lab.sh) > "$report/source.sha256"
{
 date -u; uname -a; lscpu
 printf 'runtime_image=%s\npostgres_image=%s\n' "$runtime_image" "$postgres_image"
 docker inspect "$service" --format 'memory={{.HostConfig.Memory}} swap={{.HostConfig.MemorySwap}} readonly={{.HostConfig.ReadonlyRootfs}}'
 nvidia-smi --query-gpu=name,driver_version,memory.total,memory.used,memory.free --format=csv
} > "$report/environment.txt"

run_test() {
 local pattern="$1"
 shift
 local extra=()
 if [[ "$mode" == qwen* || "$mode" == interpretation ]]; then extra+=(--volume "$root/models:/qwen-data:ro"); fi
 docker run --rm --user 10001:10001 --network "$network" \
  --read-only --tmpfs /tmp:size=256m,mode=1777 \
  --volume "$src:/src:ro" --volume "$work/decisions.test:/runner:ro" \
  --volume "$work/private:/private:ro" --volume "$report/output:/reports" \
  --volume "$report:/samples:ro" --workdir /src/internal/decisions \
  --env BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler@decision-postgres:5432/decisions_test?sslmode=disable \
  "${extra[@]}" "$@" --entrypoint /runner "$builder" -test.v -test.run "$pattern" -test.timeout 45m
}
# First prove the caller contracts against actual isolated PG with no tensor calls.
run_test '^Test' > "$report/contracts.log" 2>&1
if [[ "$mode" == taxonomy || "$mode" == qwen* || "$mode" == interpretation ]]; then
 phase="${4:-replay}"
 [[ "$phase" == replay || "$phase" == dev || "$phase" == test ]] || exit 1
 (cd "$src"; sha256sum deploy/local-models/russian-decisions-independent-v2.json internal/localmodels/decision-schema-v2.json) > "$report/taxonomy-freeze.sha256"
 sample_extra=(); if [[ "$mode" == qwen* || "$mode" == interpretation ]]; then sample_extra+=(butler-decision-qwen-v2); fi
 python3 "$src/deploy/local-models/sample-decisions.py" "$service" "${sample_extra[@]}" > "$report/$phase-cpu.jsonl" &
 sampler=$!
 extra=()
 if [[ "$mode" == qwen* || "$mode" == interpretation ]]; then
  python3 "$src/deploy/local-models/sample-decision-residency.py" > "$report/$phase-gpu.jsonl" &
  gpu_sampler=$!
  extra+=(--env BUTLER_TEST_QWEN_EVALUATION=1 --env BUTLER_TEST_QWEN_TOKEN_COUNTS=/qwen-data/token-counts.json --env BUTLER_TEST_QWEN_PINS=/qwen-data/qwen-pins.json)
  if [[ "${5:-co-resident}" == co-resident ]]; then extra+=(--env BUTLER_TEST_QWEN_CORESIDENT=1); fi
  if [[ "$mode" == qwen-cpuembed ]]; then extra+=(--env BUTLER_TEST_QWEN_EMBEDDING_CPU=1); fi
  if [[ "$phase" == dev ]]; then extra+=(--env BUTLER_TEST_DECISION_ONLY_QWEN=1); fi
 fi
 pattern='^TestActualRussianDecisionTaxonomyV2$'
 if [[ "$mode" == interpretation ]]; then
  [[ "$phase" == dev || "$phase" == test ]] || exit 1
  pattern='^TestActualInterpretationV3Comparison$'
  (cd "$src"; sha256sum deploy/local-models/russian-interpretation-heldout-v3.json deploy/local-models/interpretation-v3-freeze.json deploy/local-models/interpretation-annotation-v3.md internal/localmodels/interpretation-schema-v3.json internal/decisions/interpretation*.go) > "$report/interpretation-source.sha256"
 fi
 if [[ "$mode" == qwen-workload ]]; then pattern='^TestActualEmbeddingCPUWorkload$'; fi
 run_test "$pattern" --env BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE=/private/token \
  --env BUTLER_TEST_DECISION_PHASE="$phase" --env BUTLER_TEST_DECISION_REPORT="/reports/$phase.json" "${extra[@]}" \
  > "$report/$phase.log" 2>&1
 if [[ "$mode" == qwen-cpuembed && "$phase" == test ]]; then
  run_test '^TestActualEmbeddingCPUWorkload$' --env BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE=/private/token --env BUTLER_TEST_DECISION_REPORT=/reports/embedding-workload.json > "$report/embedding-workload.log" 2>&1
 fi
 kill "$sampler" 2>/dev/null || true; wait "$sampler" || true; sampler=''
 if [[ -n "$gpu_sampler" ]]; then kill "$gpu_sampler" 2>/dev/null || true; wait "$gpu_sampler" || true; gpu_sampler=''; fi
 docker logs "$service" > "$report/service.log" 2>&1
 printf 'Taxonomy evidence: %s\n' "$report"
 exit 0
fi
for phase in first repeat; do
 printf 'Starting %s fixed 120-attempt CPU comparison\n' "$phase"
 python3 "$src/deploy/local-models/sample-decisions.py" "$service" > "$report/$phase-cpu.jsonl" &
 sampler=$!
 run_test '^TestActualRussianDecisionComparison$' --env BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE=/private/token \
  --env BUTLER_TEST_DECISION_FULL_SCORES="$full_scores" \
  --env BUTLER_TEST_DECISION_REPORT="/reports/$phase.json" \
  --env BUTLER_TEST_DECISION_CPU_SAMPLES="/samples/$phase-cpu.jsonl" \
  > "$report/$phase.log" 2>&1
 kill "$sampler" 2>/dev/null || true; wait "$sampler" || true; sampler=''
 # Keep the same service/cgroup alive. The second pass includes cache pressure
 # and a fresh load after the configured idle unload; no flushing/restart.
 if [[ "$phase" == first ]]; then sleep 35; fi
done
nvidia-smi --query-gpu=name,memory.used,memory.free --format=csv > "$report/gpu-after.csv"
docker logs "$service" > "$report/service.log" 2>&1
printf 'Decision replay evidence: %s\n' "$report"
