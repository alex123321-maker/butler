---
name: butler-tools
description: Apply Butler tooling rules for Tool Broker ownership, runtime isolation, and stable tool contracts.
---
## What this skill covers
- Tool Broker responsibilities and policy enforcement
- Tool contracts, runtime routing, and result normalization
- Runtime isolation and container-per-tool-class behavior
- Credential-aware execution requirements for browser and HTTP tools

## Use this skill when
- A task changes Tool Broker logic
- A task adds or modifies tools or tool contracts
- A task changes runtime containers, routing, or execution policy

## Core rules
- Tools are contracts, not ad hoc helper calls
- Tool execution goes through Tool Broker
- Runtimes execute only approved work and do not own orchestration
- Sensitive tool inputs use `credential_ref`, not raw secrets

## Required references
- `docs/architecture/tool-runtime-adr.md`
- `docs/architecture/credential-management.md`
- `docs/ai/engineering-rules.md`
