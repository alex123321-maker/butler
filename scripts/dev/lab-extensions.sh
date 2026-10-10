#!/bin/sh
set -eu

# Run inside the isolated lab Go container with a read-only source snapshot.
# Parent must supply only an explicitly separate test database, never main PG.
case "${BUTLER_TEST_ACTIONAUTH_POSTGRES_URL:-}" in
  *'_test?'*) ;;
  *) echo 'isolated *_test PostgreSQL URL required' >&2; exit 1 ;;
esac
extension_bin_dir=${BUTLER_EXTENSION_E2E_OUTPUT_DIR:-/cache/extension-e2e}
mkdir -p "$extension_bin_dir"
go build -trimpath -o "$extension_bin_dir/tool-broker" ./apps/tool-broker
go test -c -o "$extension_bin_dir/tool-extension.test" ./apps/tool-extension/internal/runtime
go test -c -o "$extension_bin_dir/orchestrator-extension.test" ./apps/orchestrator/internal/app
BUTLER_EXTENSION_E2E=1 \
BUTLER_BROKER_E2E_BINARY="$extension_bin_dir/tool-broker" \
BUTLER_EXTENSION_E2E_BINARY="$extension_bin_dir/tool-extension.test" \
"$extension_bin_dir/orchestrator-extension.test" -test.v -test.timeout=180s \
  -test.run "${BUTLER_EXTENSION_E2E_TEST_REGEX:-^TestInstalledProjectExtensionRealBrokerApprovalE2E$}"
