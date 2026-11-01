#!/usr/bin/env bash
# KocFlow host maintenance. Installed as /usr/local/bin/kocflow-maint and run from /etc/cron.d/kocflow-maint:
#
#   kocflow-maint watchdog   every 3 min: restart unhealthy/slow containers, alert on low RAM / swap / disk / CPU
#   kocflow-maint cleanup    weekly: prune Docker leftovers, old logs, package caches, stale backups
#   kocflow-maint status     print what the watchdog sees right now (safe to run by hand)
#
# Alerts go to $APP_DIR/maintenance.log and, when ALERT_WEBHOOK_URL is set in .env, to that webhook
# (Slack / Discord / Mattermost style JSON). The same alert is sent at most once every 6 hours.
set -u
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

APP_DIR="${APP_DIR:-/opt/kocflow}"
COMPOSE_FILE="$APP_DIR/docker-compose.prod.yml"
ENV_FILE="$APP_DIR/.env"
STATE_DIR="$APP_DIR/.maint"
LOG_FILE="$APP_DIR/maintenance.log"
SERVICES="backend frontend zalo-bridge caddy"

RAM_WARN_PCT=15      # alert when less than this % of RAM is available
RAM_CRIT_PCT=8
SWAP_WARN_PCT=50     # alert when more than this % of swap is in use
DISK_WARN_PCT=85     # alert (and auto-clean) when the root disk is fuller than this
SLOW_MS=3000         # a /health answer slower than this counts as a strike
STRIKES=3            # consecutive strikes before the backend is restarted
ALERT_COOLDOWN=21600 # seconds

mkdir -p "$STATE_DIR"

compose() { docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"; }
now() { date +%s; }
log() { printf '%s %s\n' "$(date '+%F %T')" "$*" >> "$LOG_FILE"; }

env_value() { grep -E "^$1=" "$ENV_FILE" 2>/dev/null | tail -n1 | cut -d= -f2- | tr -d "\"'"; }

# alert <key> <message>: log always, notify at most once per cooldown per key
alert() {
	local key="$1" msg="$2" stamp="$STATE_DIR/alert-$1" last=0
	log "ALERT[$key] $msg"
	[ -f "$stamp" ] && last=$(cat "$stamp" 2>/dev/null || echo 0)
	[ $(( $(now) - last )) -lt "$ALERT_COOLDOWN" ] && return 0
	now > "$stamp"
	local url host; url="$(env_value ALERT_WEBHOOK_URL)"; host="$(hostname)"
	[ -z "$url" ] && return 0
	local text="KocFlow @ $host: $msg"
	curl -fsS -m 10 -H 'Content-Type: application/json' \
		-d "$(printf '{"text":"%s","content":"%s"}' "$text" "$text")" "$url" >/dev/null 2>&1 || log "webhook delivery failed"
}

clear_alert() { rm -f "$STATE_DIR/alert-$1"; }

mem_available_pct() { awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{ if(t>0) printf "%d", a*100/t; else print 100 }' /proc/meminfo; }
swap_used_pct()     { awk '/^SwapTotal:/{t=$2} /^SwapFree:/{f=$2} END{ if(t>0) printf "%d", (t-f)*100/t; else print 0 }' /proc/meminfo; }
disk_used_pct()     { df -P / | awk 'NR==2{gsub("%","",$5); print $5}'; }
load5_x100()        { awk '{printf "%d", $2*100}' /proc/loadavg; }

container_id() { compose ps -q "$1" 2>/dev/null | head -n1; }

check_containers() {
	local svc cid state health
	for svc in $SERVICES; do
		cid="$(container_id "$svc")"
		if [ -z "$cid" ]; then
			alert "down-$svc" "container $svc is missing; starting it"
			compose up -d "$svc" >/dev/null 2>&1
			continue
		fi
		state="$(docker inspect -f '{{.State.Status}}' "$cid" 2>/dev/null)"
		health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$cid" 2>/dev/null)"
		if [ "$state" != "running" ] && [ "$state" != "restarting" ]; then
			alert "down-$svc" "container $svc is $state; starting it"
			compose up -d "$svc" >/dev/null 2>&1
		elif [ "$health" = "unhealthy" ]; then
			alert "unhealthy-$svc" "container $svc is unhealthy; restarting it"
			docker restart "$cid" >/dev/null 2>&1
		else
			clear_alert "down-$svc"; clear_alert "unhealthy-$svc"
		fi
	done
}

# Restart the API when /health keeps failing or answering slowly (the container healthcheck only flags it, Docker never acts on it).
check_backend_latency() {
	local cid ms start strikes_file="$STATE_DIR/backend-strikes" strikes=0
	cid="$(container_id backend)"; [ -z "$cid" ] && return 0
	[ -f "$strikes_file" ] && strikes=$(cat "$strikes_file" 2>/dev/null || echo 0)
	start=$(date +%s%N)
	if docker exec "$cid" wget -qO- -T 6 http://127.0.0.1:5000/health >/dev/null 2>&1; then
		ms=$(( ($(date +%s%N) - start) / 1000000 ))
	else
		ms=99999
	fi
	if [ "$ms" -gt "$SLOW_MS" ]; then
		strikes=$((strikes + 1))
		log "backend /health took ${ms} ms (strike $strikes/$STRIKES)"
		if [ "$strikes" -ge "$STRIKES" ]; then
			alert "backend-restart" "backend was unresponsive/slow for $STRIKES checks in a row (last ${ms} ms); restarting it"
			docker restart "$cid" >/dev/null 2>&1
			strikes=0
		fi
	else
		strikes=0
	fi
	echo "$strikes" > "$strikes_file"
}

check_resources() {
	local ram swap disk load cpus
	ram=$(mem_available_pct); swap=$(swap_used_pct); disk=$(disk_used_pct); load=$(load5_x100); cpus=$(nproc)

	if [ "$ram" -lt "$RAM_CRIT_PCT" ]; then alert ram "RAM almost exhausted: only ${ram}% available"
	elif [ "$ram" -lt "$RAM_WARN_PCT" ]; then alert ram "RAM is low: ${ram}% available"
	else clear_alert ram; fi

	if [ "$swap" -gt "$SWAP_WARN_PCT" ]; then alert swap "heavy swap use: ${swap}% of swap in use (server is short of RAM)"
	else clear_alert swap; fi

	if [ "$disk" -gt "$DISK_WARN_PCT" ]; then
		alert disk "root disk is ${disk}% full; running cleanup"
		cleanup_docker
	else clear_alert disk; fi

	if [ "$load" -gt $((cpus * 200)) ]; then alert cpu "CPU overloaded: 5-minute load $((load / 100)).$((load % 100)) on $cpus cores"
	else clear_alert cpu; fi
}

cleanup_docker() {
	docker container prune -f >/dev/null 2>&1
	docker image prune -af --filter "until=72h" >/dev/null 2>&1
	docker builder prune -f --filter "until=72h" >/dev/null 2>&1
	docker network prune -f >/dev/null 2>&1
}

cmd_cleanup() {
	local before after; before=$(disk_used_pct)
	cleanup_docker
	command -v journalctl >/dev/null && journalctl --vacuum-time=7d --vacuum-size=100M >/dev/null 2>&1
	if command -v dnf >/dev/null; then dnf clean all >/dev/null 2>&1; fi
	if command -v apt-get >/dev/null; then apt-get clean >/dev/null 2>&1; fi
	# stale backups / config snapshots left behind by migrations (older than 14 days only)
	find "$APP_DIR/backups" -maxdepth 1 -type f -name 'mongo-*.gz' -mtime +14 -delete 2>/dev/null
	find "$APP_DIR" -maxdepth 1 -type f -name '.env.bak-*' -mtime +14 -delete 2>/dev/null
	find /home -maxdepth 2 -type f -name 'mongo-backup*.gz' -mtime +14 -delete 2>/dev/null
	# keep the maintenance log bounded
	if [ -f "$LOG_FILE" ] && [ "$(wc -l < "$LOG_FILE")" -gt 2000 ]; then tail -n 1000 "$LOG_FILE" > "$LOG_FILE.tmp" && mv "$LOG_FILE.tmp" "$LOG_FILE"; fi
	after=$(disk_used_pct)
	log "cleanup done: disk ${before}% -> ${after}%"
}

cmd_status() {
	echo "RAM available : $(mem_available_pct)%   (alert < ${RAM_WARN_PCT}%)"
	echo "Swap used     : $(swap_used_pct)%   (alert > ${SWAP_WARN_PCT}%)"
	echo "Disk used     : $(disk_used_pct)%   (alert > ${DISK_WARN_PCT}%)"
	echo "Load 5 min    : $(awk '{print $2}' /proc/loadavg) on $(nproc) cores"
	echo "Backend strikes: $(cat "$STATE_DIR/backend-strikes" 2>/dev/null || echo 0)/$STRIKES"
	local svc cid
	for svc in $SERVICES; do
		cid="$(container_id "$svc")"
		printf '%-12s %s\n' "$svc" "$([ -n "$cid" ] && docker inspect -f '{{.State.Status}} {{if .State.Health}}({{.State.Health.Status}}){{end}}' "$cid" || echo missing)"
	done
}

# a deploy is rolling the stack: stay out of its way (the marker goes stale after 10 min if a deploy dies)
deploying() { [ -f "$STATE_DIR/deploying" ] && [ $(( $(now) - $(stat -c %Y "$STATE_DIR/deploying" 2>/dev/null || echo 0) )) -lt 600 ]; }

case "${1:-}" in
	watchdog) deploying && exit 0; check_containers; check_backend_latency; check_resources ;;
	cleanup)  cmd_cleanup ;;
	status)   cmd_status ;;
	*) echo "usage: $0 {watchdog|cleanup|status}" >&2; exit 2 ;;
esac
