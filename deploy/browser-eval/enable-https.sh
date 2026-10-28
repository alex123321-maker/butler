#!/usr/bin/env bash
# Bounded DNS propagation wait followed by one certificate request.
set -euo pipefail
domain=${1:?test domain required}
[[ "$domain" =~ ^butler-eval[.][a-z0-9-]+[.][a-z]{2,}$ ]] || exit 2
site=/etc/nginx/sites-available/butler-browser-eval
template=/var/tmp/butler-browser-eval-https.template
test -f "$site"
test -f "$template"
test -f /var/tmp/butler-browser-eval-renew-nginx.sh
grep -Fq "server_name $domain;" "$site"
ready=false
for n in $(seq 1 30); do
 if dig @ns1.reg.ru "$domain" A +short | grep -Fxq '46.8.21.229' &&
    dig @ns2.reg.ru "$domain" A +short | grep -Fxq '46.8.21.229'; then
  ready=true
  break
 fi
 sleep 30
done
$ready || { printf 'DNS propagation not confirmed; HTTPS not enabled.\n'; exit 1; }
printf 'Authoritative DNS confirmed. Requesting one public certificate.\n'
certbot certonly --non-interactive --agree-tos --register-unsafely-without-email \
 --webroot -w /var/lib/butler-browser-eval-acme -d "$domain"
backup=/etc/nginx/sites-available/butler-browser-eval.http-only
test ! -e "$backup"
cp "$site" "$backup"
rendered=$(mktemp /var/tmp/butler-browser-eval-https.XXXXXX)
sed "s/TEST_DOMAIN/$domain/g" "$template" > "$rendered"
install -m 644 "$rendered" "$site"
if ! nginx -t; then
 cp "$backup" "$site"
 printf 'TLS configuration rejected; HTTP config restored.\n'
 exit 1
fi
systemctl reload nginx
install -m 755 /var/tmp/butler-browser-eval-renew-nginx.sh /etc/letsencrypt/renewal-hooks/deploy/butler-browser-eval
printf 'HTTPS virtual host enabled; verify publicly before native attempts.\n'
