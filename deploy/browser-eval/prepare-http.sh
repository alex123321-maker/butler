#!/usr/bin/env bash
# Run on the owner-selected fixture host, before public certificate issuance.
set -euo pipefail
domain=${1:?test domain required}
[[ "$domain" =~ ^butler-eval[.][a-z0-9-]+[.][a-z]{2,}$ ]] || exit 2
site=/etc/nginx/sites-available/butler-browser-eval
link=/etc/nginx/sites-enabled/butler-browser-eval
test ! -e "$site"
test ! -e "$link"
install -d -m 755 /var/lib/butler-browser-eval-acme/.well-known/acme-challenge
set -o noclobber
cat > "$site" <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name $domain;
    access_log off;
    error_log /dev/null crit;
    location /.well-known/acme-challenge/ {
        root /var/lib/butler-browser-eval-acme;
    }
    location / { return 404; }
}
EOF
ln -s "$site" "$link"
nginx -t
systemctl reload nginx
printf 'Dedicated ACME-only HTTP host installed; recipient is not public yet.\n'
