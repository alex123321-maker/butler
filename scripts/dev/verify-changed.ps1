[CmdletBinding()]
param()

. (Join-Path $PSScriptRoot "common.ps1")

$state = Initialize-ButlerAgentEnvironment
Assert-CommandAvailable -Name git

Push-Location $state.RepoRoot
try {
  $tracked = @(git diff --name-only --diff-filter=ACMRTUXB HEAD)
  $untracked = @(git ls-files --others --exclude-standard)
}
finally {
  Pop-Location
}

$files = @($tracked + $untracked | Where-Object { $_ -and $_.Trim().Length -gt 0 } | Sort-Object -Unique)
if ($files.Count -eq 0) {
  Write-Host "No changed files detected."
  exit 0
}

$specs = New-Object System.Collections.Generic.HashSet[string]
$tests = New-Object System.Collections.Generic.HashSet[string]

foreach ($file in $files) {
  switch -Regex ($file) {
    "^(apps/orchestrator/internal/app/(release_chain_v(?:14|15|16|17|18)_|release_chain_pending_cases|release_chain_entry_lane)|scripts/dev/scenario-release-chain-v(?:14|15|16|17|18)-|scripts/dev/release-chain-v6-final-audit/v(?:14|15|16|17|18)|deploy/acceptance/release-chain-v(?:14|15|16|17|18))" {
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/generated-converters.md")
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$tests.Add("Latest frozen fresh native entry: build then execute immediately, only the declared pending cases. Run meaningful changed-helper/current PostgreSQL and race/vet checks in parallel; final independent source, original receipts, eight budget dimensions and costs remain mandatory. Never Sync or execute a closed V14/V15/V16/V17 native root.")
      [void]$tests.Add("Use a separately frozen version-specific read-only final auditor on exact named original artifacts. Preserve V14 useful work separately from its failed normal case, all later failures and historical UNKNOWN; no completed quality/comparison rerun or duplicate-write waiver.")
      continue
    }
    "^(apps/orchestrator/internal/app/(release_chain_v(?:[789]|10|11|12|13)_|release_chain_extension_harness|release_chain_source_observer|release_chain_proposal_|release_chain_entry_lane)|scripts/dev/scenario-release-chain-v(?:[789]|10|11|12|13)-|scripts/dev/release-chain-v6-final-audit/(v(?:[89]|10|11|12|13)|proposals)|deploy/acceptance/release-chain-v(?:[789]|10|11|12|13))" {
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/generated-converters.md")
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$tests.Add("V13: five targeted actual PostgreSQL declared120 and pinned entry bootstrap checks, then separate source/runtime admission. V10/V11/V12 native roots are sealed; never Sync or execute them again. Retain V12 forty-test read/receipt source proof, historical575 and independently proved zero-native bootstrap; all current source/Stop/result/delivery guards remain.")
      [void]$tests.Add("Read-only version-specific final checker with separately frozen auditor source/binary and only individually named own-tool proposal leaves. Earlier V6/V7/V8/V9 retain their original reproduction; all failed runs and UNKNOWN stay unchanged.")
      continue
    }
    "^apps/orchestrator/internal/input/(execution_timeout_test[.]go|service[.]go)$" {
      [void]$specs.Add("docs/architecture/durable-execution-spec.md")
      [void]$tests.Add("V9 isolated PostgreSQL/race: TestExecutionTimeoutStartupBound and TestExecutionTimeoutCurrentWorkerIntegration. Explicit startup bound must preserve ordinary ten-minute default, earlier caller cancellation and durable expiry category; no network or model before admission.")
      continue
    }
    "^(docs/architecture/model-retry-policy[.]md$|scripts/dev/model-retry-lab[.]sh$|apps/orchestrator/internal/(input|orchestrator)/model_retry.*[.]go$|internal/budget/model_retry.*[.]go$)" {
      [void]$specs.Add("docs/architecture/model-retry-policy.md")
      [void]$specs.Add("docs/architecture/durable-execution-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("SSH laba: model-retry-lab.sh FRESH_ISOLATED_ROOT EXACT_COMMIT. Disposable PostgreSQL, nominal versus authority deadline, single durable retry, current Stop/source checks, known-result fresh session, equivalent confirmed-write denial, full separate reservations/NULL retention, race/vet; zero inference.")
      continue
    }
    "^(apps/orchestrator/internal/app/(release_chain_v6_|release_chain_high_)|scripts/dev/(scenario-release-chain-v6-|release-chain-v6-final-audit[-/])|deploy/acceptance/release-chain-v6-)" {
      [void]$specs.Add("docs/architecture/model-retry-policy.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$tests.Add("SSH laba: scenario-release-chain-v6-high-lab.sh FRESH_V6_ROOT prepare EXACT_CLEAN_COMMIT. Actual537 carry, retained V5 quality and failed chain, original human537 exception plus separately pinned prospective safe-timeout waiver, eight-field78/615 window and request-local stage/kernel-operation binding, disposable PostgreSQL, exact independent checker/Core gates; zero inference before independent source/runtime admission.")
      [void]$tests.Add("Fresh final production retry regression and full mandatory fault matrix before once-only V6 native chain. Successes require full measured cost; only a proved response-stream timeout followed by one safe retry may retain separately counted NULLs. Safety, Stop, original result and all old sealed windows remain unchanged.")
      continue
    }
    "^(apps/orchestrator/internal/app/release_chain_v5_|scripts/dev/(scenario-release-chain-v5-|release-chain-v5-final-audit[-/]))" {
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$specs.Add("docs/architecture/owner-goals-mandates-spec.md")
      [void]$specs.Add("docs/architecture/atomic-result-delivery.md")
      [void]$specs.Add("docs/architecture/generated-converters.md")
      [void]$tests.Add("Shared runner: use only the latest explicitly fixed fresh pending-case wrapper and clean commit. Build then execute immediately; changed helper checks and independent source/runtime review run in parallel. All old native roots, including V5 through V17, are sealed and cannot be reused.")
      [void]$tests.Add("Final version-specific independent checker verifies actual Core/Broker receipts, original result, restart/Stop/delivery and full new measured usage. Retained quality/comparison/full-fault evidence needs scoped source comparison, not hundreds of repeated pre-native checks.")
      continue
    }
    "^(deploy/acceptance/(cloud-shadow-v5-|release-chain-v5-)|scripts/dev/cloud-v5-final-audit-reproduce[.]sh$)" {
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$tests.Add("Read-only cloud-v5-final-audit-reproduce.sh against the sealed252-request native window and separately repaired auditor; exact safe SHA/size, history535/retained unknowns. Zero inference; never Sync or execute native V5.")
      continue
    }
    "^(internal/acceptance/cloud_v5_window_audit(_test)?[.]go$|scripts/dev/cloud-v5-(final-audit/|readonly-audit-lab[.]sh$))" {
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$tests.Add("SSH laba: cloud-v5-readonly-audit-lab.sh /srv/storage/butler/agents/scenario-v5-readonly-audit-fix-20261013 EXACT_COMMIT (network-none, zero inference, historical multi-request Run regression, journal tamper tests, private-byte auditor tests, race/vet)")
      [void]$tests.Add("Independently pin repaired auditor and read the same named original V5/PG leaves. Preserve the original failed audit; V5 native root is sealed, never Sync or execute.")
      continue
    }
    "^(apps/orchestrator/internal/app/(scenario_schema_dialect_|scenario_high_v5_|scenario_shadow_v5_)|internal/acceptance/cloud_(v5|schema_dialect)|cmd/cloud-observe/v5|scripts/dev/(cloud-v5-|scenario-shadow-v5|scenario-schema-dialect|fixtures/(scenario-shadow-v5|schema-dialect))|testdata/acceptance/(ru-cloud-fresh-v5|cloud-schema-dialect))" {
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("Read-only schema-dialect-final-audit-reproduce.sh against frozen a200 source; exact safe SHA and pure history283 checks. Diagnostic root is sealed: never Sync or execute.")
      [void]$tests.Add("SSH laba: scenario-shadow-v5-high-lab.sh FRESH_V5_ROOT offline EXACT_COMMIT (zero inference; baseline and compatibility must be established before signed preparation)")
      continue
    }
    "^(deploy/acceptance/schema-dialect-|scripts/dev/schema-dialect-final-audit-reproduce[.]sh$)" {
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$tests.Add("Read-only frozen schema-dialect original-byte/PG audit reproduction; verify copied artifact SHA256 and pure history283. Zero inference; diagnostic root is sealed.")
      continue
    }
    "^(deploy/acceptance/(cloud-fresh-v4-|scenario-fault-v4-)|scripts/dev/cloud-v4-final-audit-reproduce[.]sh$)" {
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$tests.Add("Read-only frozen V4 audit reproduction; verify copied content-free artifact SHA256 and pure history279. Zero inference; V4 root is sealed, never Sync or execute.")
      continue
    }
    "^apps/orchestrator/internal/app/(scenario_high_v4_|scenario_shadow_v4_|release_chain_v4_)" {
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$tests.Add("SSH laba: scenario-shadow-v4-high-lab.sh FRESH_V4_ROOT offline EXACT_COMMIT (zero inference)")
      [void]$tests.Add("SSH laba: scenario-release-chain-v4-high-lab.sh FRESH_V4_CHAIN_ROOT prepare EXACT_COMMIT (zero inference; native requires actual quality PASS)")
      continue
    }
    "^(internal/acceptance/|cmd/cloud-observe/|testdata/acceptance/|scripts/dev/(cloud-v4-review/|scenario-shadow-v4|scenario-release-chain-v4|fixtures/scenario-shadow-v4))" {
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/acceptance/...")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./cmd/cloud-observe/...")
      [void]$tests.Add("SSH laba: fresh current protocol root offline EXACT_COMMIT (zero inference); V1–V4 roots are sealed and must never be synced or executed again")
      continue
    }
    "^(scripts/dev/scenario-shadow-v3-high-lab[.]sh$|scripts/dev/fixtures/scenario-shadow-v3-|apps/orchestrator/internal/app/scenario_shadow_v3_.*[.]go$|internal/acceptance/cloud_v3.*[.]go$|testdata/acceptance/ru-cloud-fresh-v3[.]json$)" {
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/scenario-shadow-v3-high-lab.sh /srv/storage/butler/agents/scenario-shadow-v3-high-20261012 offline <exact-clean-commit> before independent whole-corpus review/signing; prepare only after signed freeze. Actual PG baseline248/shared252-new cap,30 required admission/sampler names, separate SQL parent/usage export probe, race/vet, zero inference. Execute is separately sealed; old V1/V2 failures, spending and NULL usage remain immutable.")
      continue
    }
    "^(scripts/dev/provider-stage-lab[.]sh$|internal/transport/diagnostic_stage(_test)?[.]go$|internal/transport/antigravity/diagnostic_stage_test[.]go$)" {
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("ssh laba: scripts/dev/provider-stage-lab.sh <isolated-agent-root> after Sync. Request-local allowlisted authentication/serialization/HTTP/stream diagnostics, High/Tiered synthetic failure isolation, race/vet, zero inference; no credential contents or historical backfill.")
      continue
    }
    "^(scripts/dev/scenario-shadow-v2-high-lab[.]sh$|apps/orchestrator/internal/app/scenario_shadow_v2_.*[.]go$|internal/acceptance/cloud_v2.*[.]go$|testdata/acceptance/ru-cloud-fresh-v2[.]json$)" {
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/scenario-shadow-v2-high-lab.sh /srv/storage/butler/agents/scenario-shadow-v2-high-20261012 prepare <exact-clean-commit> after independent V2 corpus/schema/policy review and freeze. Zero-native actual PG immutable baseline228/shared252-new cap, restart/expiry/contention, synthetic safe diagnostics, complete generation/JSON bounds, race/vet. Execute is separately sealed; no resampling, V1 mutation or automatic repeat.")
      continue
    }
    "^(apps/orchestrator/internal/app/(release_chain_.*|scenario_release_chain_.*|verified_task_.*)[.]go$|apps/orchestrator/internal/task/(verified_notifications|store|controller|task)[.]go$|migrations/069_verified_task_notifications[.]|scripts/dev/scenario-release-chain.*[.]sh$)" {
      [void]$specs.Add("docs/architecture/durable-tasks-spec.md")
      [void]$specs.Add("docs/architecture/owner-goals-mandates-spec.md")
      [void]$specs.Add("docs/architecture/atomic-result-delivery.md")
      [void]$specs.Add("docs/architecture/initiative-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/scenario-release-chain-lab.sh <isolated-agent-root> kernel <exact-clean-commit>, then scenario-release-chain-process-lab.sh <isolated-agent-root> chain <exact-clean-commit>. Actual PG/Broker/runtime and one loopback recipient, current owner/source/lease guards, restart/Stop/delivery faults, race/vet; zero native calls. Re-run full mandatory fault matrix on the same combined source.")
      continue
    }
    "^(scripts/dev/scenario-shadow-high-lab[.]sh$|apps/orchestrator/internal/app/scenario_shadow_.*[.]go$|internal/transport/antigravity/model_version_test[.]go$)" {
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/observation-evaluation-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/scenario-shadow-high-lab.sh /srv/storage/butler/agents/scenario-shadow-high-20261012 prepare <exact-clean-commit>. Actual isolated PG admission/reopen/expiry and synthetic sampler/model-version/scorer checks, race/vet, zero native calls. Execute is a separately sealed baseline45 + at most252 High-only shadow window; never automatically repeat execute.")
      continue
    }
    "^(scripts/dev/scenario-corpus-lab[.]sh$|cmd/command-observe/|internal/acceptance/command_suite(_test)?[.]go$|testdata/acceptance/ru-command-proposals-v1[.]json$)" {
      [void]$specs.Add("docs/architecture/persistent-assistant-spec.md")
      [void]$specs.Add("docs/architecture/action-authorization-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/scenario-corpus-lab.sh <isolated-agent-root> narrow <exact-commit> then broad after Sync. Observation-only45-case contract supplement, opaque inputs, original witnesses and exact release pins, race/vet; no model/execution or independent semantic holdout claim.")
      continue
    }
    "^(scripts/dev/scenario-fault-matrix-lab[.]sh$|internal/actionauth/(postgres|dispatch_source_context_integration_test)[.]go$|apps/orchestrator/internal/app/(scenario_fault_matrix_integration_test|converter_workers_e2e_test)[.]go$)" {
      [void]$specs.Add("docs/architecture/action-authorization-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/generated-converters.md")
      [void]$tests.Add("ssh laba: scripts/dev/scenario-fault-matrix-lab.sh <isolated-agent-root> authority-all <exact-commit> then regression after Sync. Actual PG current read-proof/expiry and ordinary converter workers/verified task/frozen-notice faults, race/vet; synthetic channel, zero inference, full mandate-to-one-notice chain remains pending.")
      continue
    }
    "^(scripts/dev/tool-turn-usage-lab[.]sh$|internal/transport/(events|usage_test)[.]go$|internal/transport/antigravity/usage_test[.]go$|apps/orchestrator/internal/orchestrator/budget(_usage_integration_test)?[.]go$)" {
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/tool-turn-usage-lab.sh <isolated-agent-root> all after Sync. Synthetic Tiered/High SSE and actual PG per-request usage, duplicate/conflict/cancel/source guards, race/vet; zero inference or historical backfill.")
      continue
    }
    "^(scripts/dev/scenario-native-history-lab[.]sh$|apps/orchestrator/internal/app/scenario_native_history_test[.]go$)" {
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/scenario-native-history-lab.sh <isolated-agent-root> after Sync. Fifteen immutable public reports carry all45 attempts, known20/NULL25; network-none/race/vet, no counter, import, allowance or inference.")
      continue
    }
    "^(scripts/dev/initiative-high-window-lab[.]sh$|apps/orchestrator/internal/app/initiative_high_window_test[.]go$)" {
      [void]$specs.Add("docs/architecture/initiative-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("ssh laba: scripts/dev/initiative-high-window-lab.sh <isolated-agent-root> after Sync. Fourteen fixed prior reports carry41 attempts into a separate at-most4-new/45-total/18-minute window; no inference or auth imports in this preparation.")
      continue
    }
    "^(scripts/dev/initiative-high-lab[.]sh$|apps/orchestrator/internal/app/initiative_high_provider_test[.]go$)" {
      [void]$specs.Add("docs/architecture/initiative-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("ssh laba: scripts/dev/initiative-high-lab.sh /srv/storage/butler/agents/initiative-high-20261012 prepare. Actual Core/Broker/channel component, positive owning High SQL route without inference, fourteen-pin counters, race/vet and fixed TLS executable. Execute only once after clean-source preparation; baseline41 + at most4 native calls.")
      continue
    }
    "^(scripts/dev/delegation-final-high-lab[.]sh$|scripts/dev/delegation-final-tls/|apps/orchestrator/internal/app/delegation_high_final.*_test[.]go$)" {
      [void]$specs.Add("docs/architecture/delegation-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/delegation-final-high-lab.sh /srv/storage/butler/agents/delegation-final-high-20261012 prepare after isolated Sync with clean committed release. Thirteen immutable history pins carry39 into a separate two-call41 ceiling; exact fixed-host two-lane TLS gate has no auth or HTTP generation. Old windows/caps stay sealed.")
      continue
    }
    "^(scripts/dev/delegation-remainder-high-lab[.]sh$|apps/orchestrator/internal/app/delegation_high_remainder.*_test[.]go$)" {
      [void]$specs.Add("docs/architecture/delegation-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/delegation-remainder-high-lab.sh /srv/storage/butler/agents/delegation-remainder-high-v2-20261012 prepare after isolated Sync with clean committed release. Mixed fixture declares scripted parents/width1 and only2 native High width2 child calls, carrying37 to original39 ceiling. All old windows remain sealed.")
      continue
    }
    "^(scripts/dev/delegation-acceptance-high-lab[.]sh$|scripts/dev/fixtures/delegation-high-conflict-20261012[.]json$|apps/orchestrator/internal/app/delegation_full_high.*_test[.]go$)" {
      [void]$specs.Add("docs/architecture/delegation-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/delegation-acceptance-high-lab.sh /srv/storage/butler/agents/delegation-acceptance-high-20261012 prepare after isolated Sync, with the clean committed BUTLER_DELEGATION_NATIVE_RELEASE (real PostgreSQL/Broker/owner verification, race/vet, zero inference). New owner-authorized one-use execute carries27 historical calls and has12-new/39-total ceiling; never reopen earlier windows.")
      continue
    }
    "^(scripts/dev/delegation-high-lab[.]sh$|apps/orchestrator/internal/app/delegation_high_window_test[.]go$)" {
      [void]$specs.Add("docs/architecture/delegation-contract.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: scripts/dev/delegation-high-lab.sh /srv/storage/butler/agents/delegation-high-20261012 prepare after isolated Sync, with BUTLER_DELEGATION_NATIVE_RELEASE set to the committed SHA (real disposable PostgreSQL, integrated regressions, race/vet, zero inference). The single High execute window is permanently sealed after attempts23-27; never reopen it.")
      continue
    }
    "^(scripts/dev/antigravity-compare/|scripts/dev/antigravity-compare-lab[.]sh$|scripts/dev/antigravity-high-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("ssh laba: scripts/dev/antigravity-compare-lab.sh /srv/storage/butler/agents/antigravity-compare tests after isolated Sync (actual disposable PG, race, vet; zero inference). inspect blocks generation; execute requires separately recorded owner authorization and cannot reopen its marker.")
      [void]$tests.Add("ssh laba: scripts/dev/antigravity-high-lab.sh /srv/storage/butler/agents/antigravity-high tests after isolated Sync (actual disposable PG, race, vet, zero inference). inspect/probe perform no generation. execute is a separately recorded one-use window carrying21 historical attempts.")
      continue
    }
    "^(scripts/dev/antigravity-wire/|scripts/dev/antigravity-wire-lab[.]sh$|internal/transport/antigravity/)" {
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("ssh laba: scripts/dev/antigravity-wire-lab.sh in a new isolated agent root after Sync; tests, default, and aicode modes use --network none, synthetic credentials and no operator profile (zero inference)")
      continue
    }
    "^(apps/tool-workspace/|apps/tool-broker/internal/app/workspace|apps/orchestrator/internal/app/workspace|internal/(config/workspace|workspaceexec/health)|deploy/docker-compose[.]workspaces|docker/tool-workspace/)" {
      [void]$specs.Add("docs/architecture/isolated-code-workspaces.md")
      [void]$specs.Add("docs/architecture/action-authorization-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/workspace-bootstrap/current/scripts/dev/workspace-lab.sh /srv/storage/butler/agents/workspace-bootstrap bootstrap' then all after isolated source Sync (actual Broker/runtime applications, private Unix/loopback, PG/files and scheduled erasure, race and vet; zero inference)")
      continue
    }
    "^(apps/tool-broker/internal/broker/workspaces?[^/]*[.]go$|internal/workspaceexec/|internal/actionauth/workspace|migrations/067_workspace_dispatches[.]|scripts/dev/workspace-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/isolated-code-workspaces.md")
      [void]$specs.Add("docs/architecture/action-authorization-contract.md")
      [void]$specs.Add("docs/architecture/resource-lease-contract.md")
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/workspace-broker-runtime/current/scripts/dev/workspace-lab.sh /srv/storage/butler/agents/workspace-broker-runtime broker' then all after isolated source Sync (actual Broker/runtime, disposable PG/files, race and vet; zero inference)")
      continue
    }
    "^(internal/memory/candidatecontrol/|internal/memory/catalog/worker_candidates|internal/(config|actionauth)/memory_candidate|apps/(orchestrator|tool-broker)/internal/(app|broker)/memory_candidate)" {
      [void]$specs.Add("docs/architecture/versioned-memory-sources.md")
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$specs.Add("docs/architecture/action-authorization-contract.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/memory-proposal-control/current/scripts/dev/memory-candidate-lab.sh /srv/storage/butler/agents/memory-proposal-control full' (actual Broker app, disposable PostgreSQL, pool2, race, zero inference)")
      continue
    }
    "^(internal/memory/catalog/(access_deadline|extraction|store)[^/]*[.]go$|internal/memory/jobs/extraction_wait_integration_test[.]go$|scripts/dev/memory-candidate-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$specs.Add("docs/architecture/versioned-memory-sources.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/memory-candidate-admission/current/scripts/dev/memory-candidate-lab.sh /srv/storage/butler/agents/memory-candidate-admission full' (disposable PostgreSQL, pool2, actual source/dedup waits, zero inference)")
      continue
    }
    "^(apps/tool-broker/internal/(app/guarded_tab|broker/guarded_tab_resources|runtimeclient/)|internal/resourceexec/single_tab_(contracts|health)|scripts/dev/guarded-tab-runtime-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/single-tab-production-composition.md")
      [void]$specs.Add("docs/architecture/resource-lease-contract.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$specs.Add("docs/architecture/action-authorization-contract.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/guarded-tab-runtime/current/scripts/dev/guarded-tab-runtime-lab.sh /srv/storage/butler/agents/guarded-tab-runtime narrow' then full after isolated Sync (actual App.New/Run, PG65, private readiness and resource arbitration; zero inference)")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/resource-runtime-bridge/current/scripts/dev/single-tab-resource-lab.sh /srv/storage/butler/agents/resource-runtime-bridge' after isolated Sync (installed native host/extension/Chromium readiness and CAS)")
      continue
    }
    "^(internal/providerauth/|cmd/provider-auth/|scripts/dev/provider-origin-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/provider-origin-audit/current/scripts/dev/provider-origin-lab.sh /srv/storage/butler/agents/provider-origin-audit narrow' after isolated Sync; full adds actual encrypted PG, race, vet and operator build without inference")
      continue
    }
    "^(internal/delegation/attempt[.]go$|apps/orchestrator/internal/(app|orchestrator)/delegation_attempt.*[.]go$|apps/orchestrator/internal/task/delegation_attempts[.]go$|scripts/dev/delegation-attempts-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/delegation-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/versioned-memory-sources.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/delegation-attempts/current/scripts/dev/delegation-attempts-lab.sh /srv/storage/butler/agents/delegation-attempts' after isolated Sync (disposable PG/pool2, owning parent pipeline, current source release and recovery/race; no model requests)")
      continue
    }
    "^(internal/transport/ollama/|deploy/local-generative/|scripts/dev/local-generative-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: go test -race ./internal/transport/ollama and go vet ./internal/transport/ollama in the isolated read-only source snapshot; live observation is separately opt-in and bounded")
      continue
    }
    "^(apps/orchestrator/internal/app/delegation_admission.*[.]go$|apps/tool-broker/internal/broker/delegation.*[.]go$|migrations/061_delegation_admission[.]|deploy/docker-compose[.]delegation[.]yml$|scripts/dev/delegation-admission-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/delegation-contract.md")
      [void]$specs.Add("docs/architecture/action-authorization-contract.md")
      [void]$specs.Add("docs/architecture/versioned-memory-sources.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/durable-tasks/current/scripts/dev/delegation-admission-lab.sh /srv/storage/butler/agents/durable-tasks' after isolated source Sync (actual private Broker, disposable PostgreSQL, pool2 and race)")
      continue
    }
    "^(internal/singletabcontrol/|internal/actionauth/input_claim|apps/browser-bridge/|apps/orchestrator/internal/singletab/|apps/tool-broker/internal/broker/(single_tab|guarded_tab_request)|web/extensions/chromium-butler/|migrations/(063_single_tab_resource_binding|064_guarded_tab_owner_choices|065_guarded_tab_request_dispatch)[.]|scripts/dev/(single-tab-resource|guarded-tab-request)-lab[.]sh$|apps/orchestrator/internal/(api|app|orchestrator)/guarded_tab|apps/orchestrator/internal/channel/telegram/guarded_tab|internal/config/guarded_tab|web/components/GuardedTabChoice[.]vue$)" {
      [void]$specs.Add("docs/architecture/single-tab-resource-boundary.md")
      [void]$specs.Add("docs/architecture/resource-lease-contract.md")
      [void]$specs.Add("docs/architecture/action-authorization-contract.md")
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$specs.Add("docs/architecture/single-tab-production-composition.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/guarded-tab-request/current/scripts/dev/guarded-tab-request-lab.sh /srv/storage/butler/agents/guarded-tab-request narrow' then full (actual private Broker process, PG/pool2, no inference)")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/resource-runtime-bridge/current/scripts/dev/single-tab-resource-lab.sh /srv/storage/butler/agents/resource-runtime-bridge' after isolated Sync (actual PG/pool2, private native host and synthetic Chromium extension)")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/task-observations/current/scripts/dev/task-observations-lab.sh /srv/storage/butler/agents/task-observations' after isolated Sync (actual waiting owner HTTP/PG and schema validation)")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/task-observations/current/scripts/dev/task-view-lab.sh /srv/storage/butler/agents/task-observations' after isolated Sync (owner choice Chromium UI)")
      continue
    }
    "^(internal/resources/|internal/resourceexec/|migrations/(058_execution_resource_leases|062_resource_dispatches)[.]|scripts/dev/resource-runtime-lab[.]sh$|apps/tool-broker/internal/broker/resources[.]go$|apps/tool-broker/internal/broker/resource_integration_test[.]go$|internal/actionauth/resource_claim[.]go$|internal/budget/tool_deadline[.]go$)" {
      [void]$specs.Add("docs/architecture/resource-lease-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("ssh laba: targeted Go race tests ./internal/resources ./internal/workspace with isolated *_test PostgreSQL; Sync the source snapshot first")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/resource-runtime-bridge/current/scripts/dev/resource-runtime-lab.sh /srv/storage/butler/agents/resource-runtime-bridge' after isolated Sync (actual PG/pool2, Broker/private runtime and Chromium)")
      continue
    }
    "^(internal/delegation/|migrations/057_task_delegations[.]|apps/orchestrator/internal/(app|orchestrator)/delegation.*[.]go$|apps/orchestrator/internal/task/delegation.*[.]go$|apps/orchestrator/internal/input/bounded_admission[.]go$|apps/orchestrator/internal/session/admission[.]go$|scripts/dev/delegation-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/delegation-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$tests.Add("ssh laba: targeted Go race tests ./internal/delegation ./internal/actionauth ./internal/budget ./apps/orchestrator/internal/task ./apps/tool-broker/internal/broker; Sync first")
      [void]$tests.Add("ssh laba: selected child context and recovery ./internal/memory/catalog ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app with isolated PostgreSQL; Sync first")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/durable-tasks/current/scripts/dev/delegation-lab.sh /srv/storage/butler/agents/durable-tasks' (disposable PostgreSQL, pool2 and manager race proof)")
      continue
    }
    "^(internal/workspace/|internal/workspaceexec/|migrations/(060_isolated_code_workspaces|066_workspace_source_provenance)[.]|scripts/dev/workspace-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/isolated-code-workspaces.md")
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/agent-security-boundaries.md")
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/durable-tasks/current/scripts/dev/workspace-lab.sh /srv/storage/butler/agents/durable-tasks' after isolated source Sync")
	  continue
	}
    "^(apps/orchestrator/internal/orchestrator/memory_source_guard.*[.]go$|internal/budget/continuation[.]go$)" {
      [void]$specs.Add("docs/architecture/generated-converters.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: isolated PostgreSQL go test -race ./internal/budget ./apps/orchestrator/internal/orchestrator; source checks must retain revocation, cancellation and finite deadline tests")
      continue
    }
    "^(internal/generatedtools/|apps/tool-converter/|docker/tool-converter/|migrations/05[2345]_|apps/orchestrator/internal/.*/.*converter|apps/tool-broker/internal/broker/converter.*[.]go$|deploy/docker-compose[.](converters|converter-delegation)[.]yml$|scripts/dev/lab-converter-container[.]sh$)" {
      [void]$specs.Add("docs/architecture/generated-converters.md")
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba: actual isolated PostgreSQL go test -race ./internal/generatedtools/... ./apps/tool-converter/... ./internal/actionauth/... ./internal/skills/... ./apps/tool-broker/... ./apps/orchestrator/internal/goal")
      [void]$tests.Add("ssh laba: scripts/dev/lab-converter-container.sh /srv/storage/butler/agents/browser-sessions after explicit Sync")
      continue
    }
    "^(internal/acceptance/|cmd/acceptance/|testdata/acceptance/|scripts/dev/fault-lab[.]|apps/orchestrator/internal/input/process_fault_integration_test[.]go$)" {
      [void]$specs.Add("docs/ai/fault-scenario-acceptance.md")
      [void]$specs.Add("docs/architecture/durable-execution-acceptance.md")
      [void]$specs.Add("docs/architecture/owner-goals-mandates-acceptance.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/fault-lab.ps1 -Profile Full -RemoteRoot /srv/storage/butler/agents/durable-tasks")
      continue
    }
    "^(apps/orchestrator/internal/initiative/|apps/orchestrator/internal/api/initiatives.*[.]go$|migrations/040_initiative_schedules[.])" {
      [void]$specs.Add("docs/architecture/initiative-contract.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/lab.ps1 -RemoteRoot /srv/storage/butler/agents/initiatives -Action Integration -Packages ./apps/orchestrator/internal/initiative")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/lab.ps1 -RemoteRoot /srv/storage/butler/agents/initiatives -Action Race -Packages ./apps/orchestrator/internal/initiative")
      continue
    }
    "^(internal/skills/|migrations/051_|apps/orchestrator/internal/.*/skill|apps/tool-broker/internal/broker/skills[.]go$)" {
      [void]$specs.Add("docs/architecture/reviewed-skills.md")
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$tests.Add("ssh laba: actual isolated PostgreSQL go test -race ./internal/skills/... ./apps/tool-broker/... ./apps/orchestrator/internal/orchestrator")
      continue
    }
    "^(internal/extensions/|apps/tool-extension/|docker/tool-extension/|deploy/extensions/|deploy/docker-compose[.]extensions[.]yml$|scripts/dev/lab-extensions[.]sh$|scripts/dev/lab-extension-container[.]sh$)" {
      [void]$specs.Add("docs/architecture/reviewed-extension-lifecycle.md")
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("ssh laba: go test ./internal/extensions/... ./apps/tool-extension/... with isolated *_test PostgreSQL")
      [void]$tests.Add("ssh laba: scripts/dev/lab-extensions.sh with own read-only snapshot and test database")
      continue
    }
    "^(internal/decisions/|deploy/local-models/(russian-decisions[.]json|verification-decisions|verify-decisions-lab[.]sh|sample-decisions[.]py))" {
      [void]$specs.Add("docs/architecture/local-decision-signals.md")
      [void]$specs.Add("docs/architecture/local-model-service.md")
      [void]$specs.Add("docs/architecture/versioned-memory-sources.md")
      [void]$tests.Add("Targeted Go race tests: ./internal/decisions with isolated BUTLER_TEST_MEMORY_POSTGRES_URL on laba")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/local-models/current/deploy/local-models/verify-decisions-lab.sh /srv/storage/butler/agents/local-models' (operator evaluation catalog/window required)")
      continue
    }
    "^(internal/memory/extraction/|internal/memory/catalog/extraction|migrations/043_memory_extraction_dedup[.]|deploy/local-models/(russian-extraction[.]json|verification-structured-extraction|verify-extraction-lab[.]sh))" {
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$specs.Add("docs/architecture/structured-memory-extraction.md")
      [void]$specs.Add("docs/architecture/versioned-memory-sources.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/local-models/current/deploy/local-models/verify-extraction-lab.sh /srv/storage/butler/agents/local-models'")
      [void]$tests.Add("Targeted Go race tests: ./internal/memory/extraction ./internal/memory/catalog ./internal/memory/jobs with isolated BUTLER_TEST_MEMORY_POSTGRES_URL on laba")
      continue
    }
    "^(internal/memory/semantic/|migrations/(041_semantic_memory_indexes|042_hybrid_retrieval_indexes)[.]|deploy/local-models/(retrieval-quality[.]json|verification-(retrieval|hybrid)-quality[.]json|verify-quality-lab[.]sh))" {
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$specs.Add("docs/architecture/local-model-service.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/local-models/current/deploy/local-models/verify-quality-lab.sh /srv/storage/butler/agents/local-models'")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/local-models/current/deploy/local-models/verify-quality-lab.sh /srv/storage/butler/agents/local-models hybrid'")
      continue
    }
    "^(apps/local-models/|internal/localmodels/|internal/gen/localmodels/|proto/localmodels/|deploy/local-models/|docker/local-models/|deploy/docker-compose[.]local-models[.]lab[.]yml$|scripts/dev/local-models(?:-capabilities)?-smoke[.]go$)" {
      [void]$specs.Add("docs/architecture/local-model-service.md")
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/lab.ps1 -RemoteRoot /srv/storage/butler/agents/local-models -Action Test -Packages ./internal/localmodels/...")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/lab.ps1 -RemoteRoot /srv/storage/butler/agents/local-models -Action Test -Packages ./apps/local-models/...")
      continue
    }
    "^(apps/orchestrator/internal/task/owner_(overview|auxiliary).*[.]go$|apps/orchestrator/internal/api/(task_observations_test|tasks_control|tasks_v2)[.]go$|apps/orchestrator/internal/run/task_(read|risk_integration_test)[.]go$|scripts/dev/task-observations-lab[.]sh$|web/(components/TaskOwnerControls[.]vue|entities/task/api[.]ts|tests/e2e/task-owner-controls[.]spec[.]ts)$)" {
      [void]$specs.Add("docs/architecture/task-owner-controls-spec.md")
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$specs.Add("docs/architecture/execution-budget-contract.md")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/task-observations/current/scripts/dev/task-observations-lab.sh /srv/storage/butler/agents/task-observations' after isolated Sync")
      [void]$tests.Add("ssh laba 'bash /srv/storage/butler/agents/task-observations/current/scripts/dev/task-view-lab.sh /srv/storage/butler/agents/task-observations' after isolated Sync")
      continue
    }
    "^apps/orchestrator/" {
      [void]$specs.Add("docs/architecture/run-lifecycle-spec.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/orchestrator/...")
      continue
    }
    "^apps/browser-bridge/" {
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/browser-bridge/...")
      continue
    }
    "^apps/tool-broker/" {
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/tool-broker/...")
      continue
    }
    "^apps/tool-browser/" {
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/tool-browser/...")
      continue
    }
    "^apps/tool-browser-local/" {
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/tool-browser-local/...")
      continue
    }
    "^apps/tool-http/" {
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/tool-http/...")
      continue
    }
    "^apps/tool-webfetch/" {
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/tool-webfetch/...")
      continue
    }
    "^apps/tool-doctor/" {
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./apps/tool-doctor/...")
      continue
    }
    "^internal/memory/" {
      [void]$specs.Add("docs/architecture/memory-model.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/memory/...")
      continue
    }
    "^internal/transport/" {
      [void]$specs.Add("docs/architecture/model-transport-contract.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/transport/...")
      continue
    }
    "^internal/credentials/" {
      [void]$specs.Add("docs/architecture/credential-management.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/credentials/...")
      continue
    }
    "^internal/config/" {
      [void]$specs.Add("docs/architecture/config-layering.md")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1 -Packages ./internal/config/...")
      continue
    }
    "^web/" {
      [void]$tests.Add("cd web; npm run typecheck")
      [void]$tests.Add("cd web; npm run lint")
      [void]$tests.Add("cd web; npm run test:e2e")
      continue
    }
    "^proto/" {
      [void]$tests.Add("make proto")
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1")
      continue
    }
    "^migrations/" {
      [void]$tests.Add("pwsh -NoProfile -File scripts/dev/test-go.ps1")
      continue
    }
  }
}

Write-Host "Changed files:" -ForegroundColor Cyan
foreach ($file in $files) {
  Write-Host ("- {0}" -f $file)
}

if ($specs.Count -gt 0) {
  Write-Host ""
  Write-Host "Relevant specs:" -ForegroundColor Cyan
  foreach ($spec in $specs) {
    Write-Host ("- {0}" -f $spec)
  }
}

if ($tests.Count -gt 0) {
  Write-Host ""
  Write-Host "Suggested verification:" -ForegroundColor Cyan
  foreach ($test in $tests) {
    Write-Host ("- {0}" -f $test)
  }
}
