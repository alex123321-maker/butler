# Catalog DATA reader

This package implements bounded source/fragment reads for the owning
Orchestrator's private Broker endpoint. It never authenticates an owner from model
arguments, executes a tool, changes permissions, accepts a fact or contacts an
external backend. The caller supplies its dispatch transaction and repeats
current guards before committing. Catalog AccessTx remains the source/ancestor
authorization boundary.

See [read contracts](../readcontrol/README.md) for limits and current search scope.
