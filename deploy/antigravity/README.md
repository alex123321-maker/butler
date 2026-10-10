# Google Antigravity with Google AI Pro

Butler's `antigravity` adapter uses the **official Antigravity CLI**, authenticated
with a personal Google account. Google AI Pro is a supported Antigravity plan;
Gemini API keys and Google Cloud consumption billing are separate access paths.
The SDK quickstart uses Gemini API/Vertex credentials and does not establish a
Google AI Pro subscription login for Butler.

This is an opt-in adapter pending a real logged-in smoke on the lab. CLI 1.3.3
advertises a nonempty native toolkit even with this agent's empty tool list, so
the lab guard rejects it and the adapter must remain disabled. Native Go API
transport is being implemented to retain Butler's exclusive tool execution.
Contract tests
exercise a fake subprocess; they do not prove the Google backend accepted a model
request or that the installed CLI honors the empty toolkit. Do not activate it
until the real `init.tools=[]` check and answer/tool-proposal roundtrip succeed.

## Model-only boundary

The CLI runs in a fresh private temporary workspace containing only the embedded
`butler-model` agent. Its toolkit, MCP servers, skills, plugins, and subagents are
disabled. Slash-command expansion is disabled. A dedicated CLI profile uses the
checked-in deny-all action policy; global customizations are rejected. CLI 1.3.3
rewrites settings by omitting empty/default fields, so Butler checks the effective
`strict`, sandbox, and required deny rules and rejects extra permissions. The adapter
requires the CLI's `init` event to name that agent and advertise an explicit empty
toolkit, and rejects native tool/delegation events or protocol changes.

Each invocation returns one schema-constrained `answer` or `tool_call` decision.
The adapter converts a proposal to Butler's normal transport event. Orchestrator
and Tool Broker apply their usual validation/approval/credential policy, execute
the tool, and submit its result back to the adapter. The CLI never receives an
executable Butler tool. Butler messages and results are replayed for the next
model turn; provider conversation IDs are not reused or stored as Butler memory.
This first version supports one proposal per turn and no assistant text deltas.

The process inherits only its actual home, basic locale/path/SSL settings, and SSH
connection marker. It receives no service database URLs or API keys. Credentials
remain inside the official CLI profile/keyring boundary; Butler neither reads nor
imports its token files. Raw CLI diagnostics/errors are excluded from transport
events and model context. Settings UI shows `CLI profile` without claiming that a
Google login is currently valid; validation happens on a requested model turn.

## Lab setup

The existing lab browser image supplies a non-root UID `10001` and trusted Linux
runtime. On the lab, run the script from a synced source checkout:

```sh
bash deploy/antigravity/setup-lab.sh
```

The script downloads CLI 1.3.3 native and musl binaries from the public manifest,
verifies SHA512, and keeps binary/cache/profile data under
`/srv/storage/butler/agents/antigravity-provider`. It does not alter shell profiles
or copy any existing account. The private profile is owned by UID `10001`, with
mode `0700`, outside all source snapshots.

Sign in yourself from a local terminal:

```sh
ssh -t laba 'bash /srv/storage/butler/agents/antigravity-provider/login.sh'
```

Select your personal Google account, open the official authorization URL in your
browser, and paste the authorization code directly into that SSH terminal. Never
send it through Butler/chat. Exit with `/exit` when login completes. The same
command allows `/logout`. Setup requests `useG1Credits: false`, but CLI settings
canonicalization omits this field. Check the effective account credit/overage
policy during login; an omitted field alone does not prove charging is disabled.
The login's empty read-only `/work` mount is the only permitted trusted workspace.
Model invocations use fresh separate directories and a five-minute timeout.

Run the model-only smoke after login, from the synced source checkout on the lab:

```sh
bash deploy/antigravity/smoke-lab.sh
```

It checks a real Russian text response, one real `web.fetch` proposal, and real
model continuation with an explicitly synthetic tool observation. It does not
execute a web tool or prove the full Broker/network path; that requires a separate
deployed Butler task. Its output is limited to pass/fail messages and normalized
safe errors, with no raw provider diagnostics or account material.

After the real smoke passes, add `deploy/docker-compose.antigravity.lab.yml` as the
last Compose file in the lab deployment command. It mounts the verified musl
binary read-only and the same private UID `10001` profile into Orchestrator.
Keep `BUTLER_ANTIGRAVITY_MODEL` set to a model slug available to your account
(`agy models`); the initial default is `gemini-3.8-flash-tiered`.

The active provider and model slug are managed through existing Butler settings.
Binary/profile paths are environment-only operational configuration. A missing
binary, missing login, altered policy, nonempty toolkit, invalid proposal, quota
failure, or lost stream fails the model turn without executing a native CLI tool.
There is no automatic API billing fallback.

## Primary references

- [CLI installation and SSH authentication](https://www.antigravity.google/docs/cli/install/)
- [Headless protocol and structured output](https://www.antigravity.google/docs/cli/headless/)
- [Custom agents and tool lists](https://www.antigravity.google/docs/subagents/)
- [Permission policy](https://www.antigravity.google/docs/permissions/)
- [Google AI Pro plans](https://www.antigravity.google/docs/plans/)
- [SDK API-key/Vertex quickstart](https://www.antigravity.google/docs/sdk/overview/)
