#!/usr/bin/env bash
# Installs the host maintenance job (idempotent). Run on the server as a sudoer:
#   sudo bash /opt/kocflow/install-maintenance.sh
# Called by setup-server.sh and by the deploy workflow on every release.
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/kocflow}"

install -m 0755 "$APP_DIR/maintenance.sh" /usr/local/bin/kocflow-maint

cat > /etc/cron.d/kocflow-maint <<'CRON'
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
*/3 * * * * root flock -n /run/kocflow-watchdog.lock /usr/local/bin/kocflow-maint watchdog
30 3 * * 0 root flock -n /run/kocflow-cleanup.lock /usr/local/bin/kocflow-maint cleanup
CRON
chmod 0644 /etc/cron.d/kocflow-maint

# MongoDB lives in Atlas now: the old local mongodump cron would only log errors.
rm -f /etc/cron.d/kocflow-backup

# make sure a cron daemon is actually running (crond on Oracle/RHEL, cron on Debian/Ubuntu)
systemctl enable --now crond >/dev/null 2>&1 || systemctl enable --now cron >/dev/null 2>&1 || echo "warning: could not start a cron daemon" >&2

echo "kocflow-maint installed: watchdog every 3 min, cleanup Sundays 03:30"
