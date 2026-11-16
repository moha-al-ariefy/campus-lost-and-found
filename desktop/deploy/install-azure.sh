#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 1 || "${EUID}" != 0 ]]; then
    printf 'Usage: sudo bash install-azure.sh PUBLIC_HOSTNAME\n' >&2
    exit 2
fi
host=$1
if [[ ! "$host" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*\.[a-zA-Z]{2,}$ ]]; then
    printf 'Use the full public DNS hostname without https:// or a path.\n' >&2
    exit 2
fi
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
for command in caddy curl systemctl systemd-analyze; do
    command -v "$command" >/dev/null
done
id claf >/dev/null
test -s /var/lib/claf/campus.db
sha256sum -c campus-server-stage0-linux-amd64.sha256
for name in campus-server.service campus-backup.service campus-backup.timer; do
    test -s "$name"
done
systemd-analyze calendar 'Sun *-*-01..07 02:00:00 Asia/Dubai' >/dev/null
systemd-analyze calendar 'Sun *-*-15..21 02:00:00 Asia/Dubai' >/dev/null

rollback=$(mktemp -d /var/backups/claf-deploy.XXXXXX)
caddy_config="$rollback/Caddyfile.new"
sed 's/{[$]CLAF_API_HOST}/'"$host"'/' Caddyfile > "$caddy_config"
caddy validate --adapter caddyfile --config "$caddy_config"
for path in /usr/local/bin/campus-server /etc/claf/server.env /etc/caddy/Caddyfile \
    /etc/systemd/system/campus-server.service /etc/systemd/system/campus-backup.service \
    /etc/systemd/system/campus-backup.timer; do
    if [[ -f "$path" ]]; then
        cp -a --parents "$path" "$rollback"
    fi
done
printf 'Previous files saved to %s\n' "$rollback"
trap 'printf "Installation stopped. Previous files are in %s. Check the error above.\n" "$rollback" >&2' ERR

install -d -o claf -g claf -m 0700 /var/lib/claf-backups
checkpoint=$(runuser -u claf -- mktemp -d /var/lib/claf-backups/pre-upgrade.XXXXXX)
staged=$(mktemp /usr/local/bin/.claf-upgrade.XXXXXX)
trap 'rm -f -- "$staged"' EXIT
install -m 0755 campus-server-stage0-linux-amd64 "$staged"
runuser -u claf -- env CAMPUS_DATA_DIR=/var/lib/claf \
    "$staged" -backup "$checkpoint/campus.db"

install -d -m 0755 /etc/claf
printf 'APP_ORIGIN=https://%s\nCAMPUS_SIGNUP=true\n' "$host" > "$rollback/server.env.new"
install -m 0644 "$rollback/server.env.new" /etc/claf/server.env
install -m 0755 campus-server-stage0-linux-amd64 /usr/local/bin/campus-server.new
mv -T /usr/local/bin/campus-server.new /usr/local/bin/campus-server
install -m 0644 campus-server.service campus-backup.service campus-backup.timer /etc/systemd/system/
install -m 0644 "$caddy_config" /etc/caddy/Caddyfile

systemctl daemon-reload
systemctl enable campus-server
systemctl restart campus-server
curl --fail --silent --show-error --max-time 3 --retry 8 --retry-connrefused \
    http://127.0.0.1:3000/healthz
systemctl enable --now caddy
systemctl reload caddy
systemctl start campus-backup.service
systemctl enable --now campus-backup.timer
systemctl list-timers campus-backup.timer --no-pager
printf 'Local installation complete. Test https://%s/api/setup from Windows.\n' "$host"
