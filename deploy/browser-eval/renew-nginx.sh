#!/usr/bin/env bash
# Certbot deploy hook: reload only for this test certificate, after syntax check.
set -euo pipefail
case " ${RENEWED_DOMAINS:-} " in
 *" butler-eval.report-template.ru "*) /usr/sbin/nginx -t && /usr/bin/systemctl reload nginx ;;
esac
