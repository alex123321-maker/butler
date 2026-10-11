# Function proposals (optional experiment)

This Go module evaluates a pinned FunctionGemma 270M Q8_0 CPU model against a
fixed read/diagnostic catalog. It publishes only shadow observations. It has no
Execute, task control, credential, memory acceptance or automatic routing API.
Owner-created finite `extract/function_shadow` jobs pin the complete current
catalog source graph. Each actual inference reserves a fresh shared root/mandate/
owner-day/job allowance, preserves cancellation and rechecks source revisions
after allowance contention and returned output. Secret or forged sources deny
before IPC; completed-task run leases are not reused.

The model uses the existing local-model CPU fair queue, shared RAM gate, RSS
telemetry and hard 8 GiB cgroup. A Go-owned Ollama child and its llama runner are
killed together on cancellation and idle unload. There is no external Ollama
execution lane, online inference pull or cloud fallback. Operator-only absolute
executable/model paths and immutable read-only storage binds are required.

The fixed catalog maps `check_system`, `check_database`, `read_webpage` to normal
`doctor.check_system`, `doctor.check_database`, `web.fetch` contracts. `ToProposal`
checks the current contract revision and complete schema, rejecting changed or
self-created tools. It only normalizes a proposal; current owner input, run/task
revision, scope, permission and budget checks remain in ordinary orchestration
and Broker Prepare/Authorize/Claim. Shadow DATA cannot establish those proofs.

All model calls remain disabled in the normal catalog and no core route is wired.
Unknown command/schema, ambiguous parameters, unsupported duration/chaining,
invalid model output or disabled experiment retain the ordinary controller route.
Fine-tuning and a complete live Gemini3.8Flash comparison are not implemented;
there is no production accuracy or cost-saving claim. The frozen Russian corpus
and every result/error are retained in `deploy/local-models/functiongemma-ru.json`
and its generated verification artifact. See the acceptance matrix in
`docs/architecture/function-proposals.md` before considering activation.
