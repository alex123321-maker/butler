# Owner task observations — issue 32

Commit `2d66b9a` adds the authenticated owner projection and task-page controls.
Two actual PostgreSQL tests and three targeted API tests passed under Go race
detection; affected task/API vet passed. Proof snapshot `20261011-114554-993`:
`/srv/storage/butler/agents/task-observations/reports/task-observations-20261011T044558Z-3706275`.
Raw events SHA256:
`71779baaaff857db0fb416f767470e8ba47c141e7c9c73326e3e886a8c86a5af`.

Vue typecheck, complete lint, production build and nine actual Chromium task
fixtures passed on snapshot `20261011-114656-572`. Private log:
`/srv/storage/butler/agents/task-observations/reports/task-view-20261011T044659Z.log`.
Tests cover result/delivery/uncertain-effect distinctions, amendment/stop,
incomplete shared usage, unverified child task results, no token persistence,
late-response fencing after proof changes and unavailable evidence on read failure.
No actual provider request, main deployment change or external action occurred.

This completes the missing owner view for shared budget, internal branches and
recorded contract effects. The broader issue still requires the initiative-to-
verified-repair scenario, model/resource diagnostics and deployed channel proof.
The UI does not turn a completed model answer into a verified task result or
invent a risk rating from execution status.

The task collection SQL also now reports risk `unknown`; failed/tool-running
states cannot establish danger. The actual PostgreSQL regression covers failed,
tool-running and completed runs under still-unverified active tasks. The later
owner/collection suite passed with race detection on snapshot
`20261011-121430-072` (proof `task-observations-20261011T051434Z-3803281`, events
SHA256 `74dd1c80fd537e05569de6813c2635bc68092d824bb435af9bfa5c3b2fd37999`).

The owner projection now also includes the latest 50 durable auxiliary jobs
across the owned task tree, with explicit truncation, requested provider/model,
expired deadline and obsolete task version. These are queue observations, not
runtime readiness, permission or verified success. Foreign-owner jobs and raw
purpose, source, error, claim and allowance material are excluded.
Actual PostgreSQL queue/privacy/version/truncation regressions, the complete
Orchestrator app package, owner API and waiting-selection tests passed with race
detection and affected vet on snapshot `20261011-123209-556`. Private proof:
`/srv/storage/butler/agents/task-observations/reports/task-observations-20261011T053225Z-3869451`.
Events SHA256:
`1bca2dabb54a7b16d5cd693651cd03a9c0a0272e40565454e0b4055075fc42e9`.
Vue typecheck, lint and all nine Chromium task-control fixtures passed in
`/srv/storage/butler/agents/task-observations/reports/task-view-20261011T053225Z.log`.
No actual model request or main deployment change occurred.
