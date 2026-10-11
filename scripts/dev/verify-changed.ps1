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
    "^(internal/workspace/|migrations/060_isolated_code_workspaces[.]|scripts/dev/workspace-lab[.]sh$)" {
      [void]$specs.Add("docs/architecture/isolated-code-workspaces.md")
      [void]$specs.Add("docs/architecture/tool-runtime-adr.md")
      [void]$specs.Add("docs/architecture/agent-security-boundaries.md")
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
