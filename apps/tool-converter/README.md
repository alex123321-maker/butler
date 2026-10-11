# Pure converter runtime

This Go service runs the single bounded `delimited_json_v1` declarative class.
`Develop` creates a package in a temporary workcopy; `TestPackage` applies the
independent fixed checker; `Convert` rechecks the need and produces all-or-nothing
typed JSON. No arbitrary code, path, command, URL, process, import or secret operation
is supported. It owns no orchestration, authorization or catalog decisions.

The private gRPC service requires a dedicated token from
`BUTLER_CONVERTER_RUNTIME_TOKEN_FILE` and an exact operator
`BUTLER_CONVERTER_PROFILE_JSON` (`dependency` ID/version/binary digest plus fixed
checker). Default gRPC/health ports are 9090/8080; `BUTLER_CONVERTER_TEMP_DIR` defaults
to `/tmp`. Core credentials and storage connections must not be supplied. Classified
Secret or recognized credential material is denied before filesystem work.

Use UID10001, read-only root, dropped capabilities, no-new-privileges, bounded
PID/RAM and a small noexec/nosuid/nodev tmpfs. The production catalog/Compose wiring
is pending; do not expose this runtime directly to a model. Full contracts and
acceptance limits are in [generated converters](../../docs/architecture/generated-converters.md).

The disposable lab helper `scripts/dev/lab-converter-container.sh` tests the actual
binary in a network-none container with synthetic data and a synthetic service token.
It does not start or alter the shared stack. Narrow Go checks are
`go test ./internal/generatedtools ./apps/tool-converter/...` and `go test -race`
for the same packages, run on the lab after an explicit snapshot sync.
