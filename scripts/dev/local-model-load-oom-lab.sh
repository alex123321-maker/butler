#!/usr/bin/env bash
# One approved cached Qwen3 CPU LOAD fault. No inference, download or HTTP.
set -euo pipefail
root=${1:?isolated agent root required}
commit=${2:?exact release commit required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ && "$commit" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/go.mod" ]] || exit 2
umask 077
mkdir -p "$root/reports" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
image=sha256:b86366bb528bbf7f1424435d165028497a5b69bf6ddb4fa5a87102e2b79f44fb
weight_hash=7f4030143c1c477224c5434f8272c662a8b042079a0a584f0a27a1684fe2e1fa
weight=/srv/storage/butler/agents/local-generative/model-store/blobs/sha256-$weight_hash
manifest=/srv/storage/butler/agents/local-generative/model-store/manifests/registry.ollama.ai/library/qwen3/0.6b
registry_hash=7df6b6e09427a769808717c0a93cadc4ae99ed4eb8bf5ca557c90846becea435
registry=/srv/storage/butler/agents/local-generative/model-store/blobs/sha256-$registry_hash
[[ -f "$weight" && ! -L "$weight" && "$(readlink -f "$weight")" == "$weight" && "$(stat -c %s "$weight")" == 522640096 ]]
[[ "$(sha256sum "$weight" | cut -d' ' -f1)" == "$weight_hash" ]]
manifest_hash=$(sha256sum "$manifest" | cut -d' ' -f1)
[[ "$manifest_hash" == 6f76d4346c34ba89df6e137fb661b4bb4daf322c1151b6ef0dfda981070a851f ]]
grep -Fq '"mediaType":"application/vnd.docker.distribution.manifest.v2+json","digest":"sha256:'"$registry_hash"'","size":858' "$manifest"
[[ -f "$registry" && ! -L "$registry" && "$(stat -c %s "$registry")" == 858 && "$(sha256sum "$registry" | cut -d' ' -f1)" == "$registry_hash" ]]
[[ "$(docker image inspect "$image" --format '{{.Id}}')" == "$image" ]]
runner_hash=$(sha256sum "$source_dir/scripts/dev/local-model-load-oom-lab.sh" | cut -d' ' -f1)
# Atomic one-use marker is never reopened, even after failure or interruption.
mkdir -p /srv/storage/butler/fault-acceptance-windows
marker=/srv/storage/butler/fault-acceptance-windows/issue33-cached-qwen3-load-20261012
mkdir "$marker" || { printf 'Cached model load window already sealed\n' >&2; exit 1; }
proof="$root/reports/local-model-load-oom-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof"
printf '%s\n' "$proof" > "$marker/report-path.txt"
printf '%s\n' "$commit" > "$proof/source-commit.txt"
printf '%s\n' "$runner_hash" > "$proof/runner-sha256.txt"
container="butler-cached-model-load-oom-$$-$(date -u +%s)"
created=false
cleanup() { $created && docker rm -f "$container" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
started=$SECONDS
# The cached runtime's own --help defines none as "no special loading mode",
# compared with auto/mmap/mlock. It disables special mapping, not tensor load;
# --model still supplies the full GGUF to the CPU loader. No inference is sent.
docker create --name "$container" --runtime runc --user 10001:10001 --network none --read-only \
  --memory 256m --memory-swap 256m --cpus 1 --pids-limit 64 --cap-drop ALL --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,noexec,nosuid,nodev,size=8m,uid=10001,gid=10001,mode=0700 \
  --volume "$weight:/model.gguf:ro" --env HOME=/tmp --env CUDA_VISIBLE_DEVICES=-1 \
  --env NVIDIA_VISIBLE_DEVICES=void --env LD_LIBRARY_PATH=/usr/lib/ollama \
  --entrypoint /usr/lib/ollama/llama-server "$image" \
  --model /model.gguf --load-mode none --device none --n-gpu-layers 0 --ctx-size 16384 \
  --batch-size 512 --parallel 1 --threads 1 --host 127.0.0.1 --port 11435 >/dev/null
created=true
# Assert actual Docker policy before starting the one load; no socket, source,
# database, credentials, observer server or GPU is accessible in this container.
actual=$(docker inspect --format '{{.Image}}|{{.HostConfig.Memory}}|{{.HostConfig.MemorySwap}}|{{.HostConfig.NanoCpus}}|{{.HostConfig.PidsLimit}}|{{.HostConfig.Runtime}}|{{.HostConfig.NetworkMode}}|{{.Config.User}}|{{.HostConfig.ReadonlyRootfs}}|{{len .Mounts}}|{{range .Mounts}}{{.Destination}}:{{.RW}}{{end}}|{{len .HostConfig.Devices}}|{{len .HostConfig.DeviceRequests}}|{{.HostConfig.Privileged}}|{{json .HostConfig.CapDrop}}|{{json .HostConfig.SecurityOpt}}' "$container")
[[ "$actual" == "$image|268435456|268435456|1000000000|64|runc|none|10001:10001|true|1|/model.gguf:false|0|0|false|[\"ALL\"]|[\"no-new-privileges:true\"]" ]]
printf '%s\n' "$actual" > "$proof/isolation.txt"
docker start "$container" >/dev/null
printf '1\n' > "$proof/actual-model-load-attempts.txt"
set +e
timeout 90 docker wait "$container" > "$proof/docker-wait.txt"
wait_exit=$?
set -e
elapsed=$((SECONDS-started))
[[ "$wait_exit" == 0 ]] || { printf 'Bounded model load did not finish; marker remains sealed\n' >&2; exit 1; }
exit_code=$(docker inspect --format '{{.State.ExitCode}}' "$container")
oom=$(docker inspect --format '{{.State.OOMKilled}}' "$container")
# Only public weight loader metadata is inspected; no prompt or inference occurs.
docker logs "$container" > "$proof/public-weight-loader.log" 2>&1
metadata=false
grep -q 'llama_model_loader: loaded meta data with' "$proof/public-weight-loader.log" && metadata=true
printf '{"version":"butler-cached-cpu-load-oom-v1","runner_sha256":"%s","image":"%s","weights_sha256":"%s","weights_bytes":522640096,"manifest_sha256":"%s","registry_manifest_sha256":"%s","actual_model_load_attempts":1,"actual_inference_calls":0,"actual_native_calls":0,"model_metadata_read":%s,"oom_killed":%s,"exit_code":%s,"elapsed_seconds":%s,"memory_bytes":268435456,"memory_swap_bytes":268435456,"nano_cpus":1000000000,"pids_limit":64,"runtime":"runc","network_mode":"none","user":"10001:10001","read_only_root":true,"only_read_only_weight_mount":true,"no_device_requests":true,"no_capabilities":true,"no_new_privileges":true,"cpu_only":true,"model":"qwen3:0.6b","stage":"load_before_inference","butler_kernel_received_oom":false}\n' \
  "$runner_hash" "$image" "$weight_hash" "$manifest_hash" "$registry_hash" "$metadata" "$oom" "$exit_code" "$elapsed" > "$proof/local-load-oom.json"
sha256sum "$proof/local-load-oom.json" "$proof/isolation.txt" "$proof/public-weight-loader.log" > "$proof/artifacts.sha256"
printf 'Content-free local load proof: %s\n' "$proof/local-load-oom.json"
[[ "$metadata" == true && "$oom" == true && "$exit_code" == 137 && "$elapsed" -le 90 ]]
