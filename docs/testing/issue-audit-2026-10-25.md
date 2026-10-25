# Open issue audit — 2026-10-25

Repository: `alex123321-maker/butler`. Initial default branch:
`fdc573f245e54693d38ea1d2652b61c39413aae2`. All 15 open issues and their available
comments were inspected. This is a local-work audit: no SSH, production deployment,
new provider inference, phone changes, GPU rental or deferred model experiment.
Another active agent owns the research-recovery worktree/PR150; its changes and
the primary checkout's untracked `prototypes/` were preserved.

Initial audit outcome: issue106 closed after PR151 merged as
`c8e08bfc17bc37905341f92e93bbd9271c05e7af`;14 issues remain open. Validation was
local-only:35 top-level Go/race tests and14 Chromium cases passed, plus scoped
vet, typecheck and lint. GitHub Actions and production rollout were not run.

| Issue | Result of review / remaining completion gate |
| --- | --- |
| [2](https://github.com/alex123321-maker/butler/issues/2) | Umbrella plan; several listed capabilities still lack complete acceptance. Child closures do not establish the cross-cutting product scenario. Keep open. |
| [20](https://github.com/alex123321-maker/butler/issues/20) | Bounded generated-converter process is implemented and has scripted production-process evidence. `generated-converters-acceptance.md` explicitly retains native same-task planning, independent quality/cost and final deployed acceptance. Keep open. |
| [21](https://github.com/alex123321-maker/butler/issues/21) | Independent kernel release/canary/backup/compatibility/rollback acceptance is not provided by the generated-converter path or by ordinary Git edits. Separate deployment authority and independent validation remain mandatory. Keep open. |
| [27](https://github.com/alex123321-maker/butler/issues/27) | Explicit owner deferral, failed frozen quality gate and unproved real controller benefit. Preserve open/deferred status; do not treat a broad issue sweep as permission to promote a failed model. |
| [28](https://github.com/alex123321-maker/butler/issues/28) | Same explicit deferral; human-reviewed real events/current-controller baseline/calibration are missing. Keep open/deferred. |
| [29](https://github.com/alex123321-maker/butler/issues/29) | Russian voice runs, but the frozen human-speech critical-name gate fails. A self-roundtrip or another unit test cannot qualify independent speech/listening quality. Keep open. |
| [57](https://github.com/alex123321-maker/butler/issues/57) | Existing Android ingress/attention reports retain the deferred 24-hour battery test, long offline/Doze/reboot and external mobile-route gates. A local Go rerun cannot replace those physical-device observations. Keep open. |
| [76](https://github.com/alex123321-maker/butler/issues/76) | Task-aware preference selection is a bounded implemented slice. The complete behavioral pilot, retained failure catalog and paired outcome/cost evaluation remain broader than that slice. Keep open. |
| [97](https://github.com/alex123321-maker/butler/issues/97) | Requires prospectively pinned comparable Low/High native profiles and the A/B/C/D matrix, including old/unknown spending and independent semantic assessment. No such completed local experiment was found. Keep open. |
| [98](https://github.com/alex123321-maker/butler/issues/98) | Communication umbrella; C09/full AC01–AC22 and current native behavior are not established by C08 diagnostics. Keep open. |
| [106](https://github.com/alex123321-maker/butler/issues/106) | **Closed, implemented and merged in [PR151](https://github.com/alex123321-maker/butler/pull/151).** Metadata diagnostics, real Broker schema observation, durable bounded owner questions, metrics, additive legacy migration proof and compatible drain runbook passed local C08 acceptance. See [the report](communication-diagnostics-2026-10-25.md). |
| [107](https://github.com/alex123321-maker/butler/issues/107) | Requires full native release acceptance, real Web/Telegram/owner bindings and staged enable preparation. Local synthetic PostgreSQL/Chromium checks do not replace it. Production enable remains a separately authorized operation. Keep open. |
| [115](https://github.com/alex123321-maker/butler/issues/115) | Requires preserved native predecessor/UNKNOWN accounting, selected experience/coherent composition windows and eventual owner-reviewed operational tasks. Historical negative and incomplete windows cannot be reset or replaced by a new empty local fixture. Keep open. |
| [140](https://github.com/alex123321-maker/butler/issues/140) | Real-pack multi-model experiment still needs immutable splits, blind descriptions/qrels, resource/cost evidence and independent assessment. No dataset import or inference was performed in this sweep. Keep open. |
| [147](https://github.com/alex123321-maker/butler/issues/147) | Current-request/history focus intersects the other active agent's PR150. Full native before/after remains a separate issue criterion; do not duplicate its implementation or infer a semantic pass from delivery success. Keep open here. |

These are completion gates, not a decision to cancel the remaining work. An
issue is not closed merely because a dependency merged, a model returned a final,
tests skipped, a channel ACK arrived, or an old implementation report exists.

Owning sources used for the narrow decisions include
`docs/architecture/generated-converters-acceptance.md`,
`docs/architecture/russian-voice.md`,
`docs/testing/issue-57-android.md`, `docs/testing/issue-57-attention.md`,
the communication roadmap, the issue bodies/comments and the actual current code.

## Authorized local follow-up

The owner authorized continued local implementation, unrestricted subscription
inference and a new local Gemini login. This supersedes the initial no-new-
inference boundary above; the lab/working deployment remains untouched.

- Issue140 closed through PR152 (`f3e87cec0ccfd9b824563d6f594e8268efbca5ea`)
  within the separately approved current-hardware/self-labelled scope. See
  [the closure report](emoji140-closure-2026-10-25.md); no original BF16 or
  independent blind/OCR qualification is inferred.
- Issue97 closed through PR153 (`7b113de8f19ce4ddc95e34fc457e3dbab232cb07`).
  The [local research report](effort-research-results-2026-10-25.md) retains
  all spending/unknowns and the synthetic Low/High and freshness results.
  The working High default remains unchanged.
- Issue20's bounded converter acceptance now passed on the actual local High
  provider/process path. [Final evidence](converter20-local-results-2026-10-25.md)
  maps every issue criterion; PR154 contains the fixes and raw A..F windows.
  This qualifies declarative converters, not arbitrary executable generation,
  kernel self-update, semantic owner verification or production rollout.
- The owner explicitly reconfirmed that issues27/28 remain deferred.
- Issue147/PR150/PR155 belongs to the other agent. Its work is integrated where
  needed but is not claimed as this audit's completion.

The original table is the initial snapshot, not the final current issue state.
All remaining gates retain their distinctions: device observations, native
release/owner bindings, protected release authority and independent quality
acceptance cannot be inferred from the bounded synthetic results above.
