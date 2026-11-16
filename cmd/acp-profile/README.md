# Dedicated ACP profile preparation

This operator command creates or validates only the managed custom-agent
manifest and safety settings for the experimental Antigravity ACP adapter.
It does not install a CLI, authenticate, import credentials or perform inference.
It is not a service and needs no network or database.

From the repository root, with a new absolute directory:

```sh
go run ./cmd/acp-profile -directory /srv/private/butler-acp-home
```

On Linux the new profile/directories use mode 0700 and managed files 0600.
Existing directories must already match; ordinary homes are refused and never
rewritten. Repeating the command validates the same profile without overwriting
it. Modified settings, hooks, plugins, skills, other agents, active MCP servers
or project resources fail closed. Managed files must be regular, private and
bounded; symlinks and special files are refused.

This is preparation only, not a sign-in or deployment recipe. The adapter is
Linux-only and uses an explicitly approved opaque request/time/data budget.
Native token counts remain unknown. Account route, enforced native-tool disabling
and runtime/container compatibility have separate evidence listed in the
[ADR](../../docs/architecture/antigravity-acp-adr.md). Do not copy an existing
Google profile or provide tokens in chat. Any later owner-authorized login
must take place through Google's own flow in a separately isolated runtime,
followed by revalidation. Legitimate CLI changes to managed settings also cause
refusal; the command deliberately does not silently repair them.
