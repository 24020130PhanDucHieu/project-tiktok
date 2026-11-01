#!/usr/bin/env bash
# One-time bootstrap for an Oracle Linux 9 VM (Always Free VM.Standard.E2.1.Micro, 1 GB RAM).
# Run as the default user (opc):  bash setup-server.sh
# Safe to re-run.
set -euo pipefail

APP_DIR=/opt/kocflow

echo "==> 1/6 swap (2 GB) - the VM has only 1 GB of RAM"
if [ -z "$(swapon --show --noheadings)" ]; then
  sudo fallocate -l 2G /swapfile 2>/dev/null || sudo dd if=/dev/zero of=/swapfile bs=1M count=2048
  sudo chmod 600 /swapfile
  sudo mkswap /swapfile >/dev/null
  sudo swapon /swapfile
  grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab >/dev/null
fi
echo 'vm.swappiness=20' | sudo tee /etc/sysctl.d/99-kocflow.conf >/dev/null
sudo sysctl -q --system

echo "==> 2/6 free memory: stop services this VM does not need"
for svc in oracle-cloud-agent-updater; do sudo systemctl disable --now "$svc" 2>/dev/null || true; done

echo "==> 3/6 Docker Engine + compose plugin"
if ! command -v docker >/dev/null; then
  sudo dnf -y install dnf-plugins-core >/dev/null
  sudo dnf config-manager --add-repo https://download.docker.com/linux/rhel/docker-ce.repo
  sudo dnf -y install docker-ce docker-ce-cli containerd.io docker-compose-plugin >/dev/null
fi
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER" || true
echo '{"log-driver":"json-file","log-opts":{"max-size":"10m","max-file":"3"}}' | sudo tee /etc/docker/daemon.json >/dev/null
sudo systemctl restart docker

echo "==> 4/6 firewall: open 80/443"
if systemctl is-active --quiet firewalld; then
  sudo firewall-cmd --permanent --add-service=http --add-service=https >/dev/null
  sudo firewall-cmd --reload >/dev/null
fi
# Oracle images also ship restrictive iptables rules
sudo iptables -C INPUT -p tcp --dport 80  -j ACCEPT 2>/dev/null || sudo iptables -I INPUT 5 -p tcp --dport 80  -j ACCEPT
sudo iptables -C INPUT -p tcp --dport 443 -j ACCEPT 2>/dev/null || sudo iptables -I INPUT 5 -p tcp --dport 443 -j ACCEPT
command -v netfilter-persistent >/dev/null && sudo netfilter-persistent save || sudo sh -c 'iptables-save > /etc/sysconfig/iptables' 2>/dev/null || true

echo "==> 5/6 app directory"
sudo mkdir -p "$APP_DIR"
sudo chown "$USER":"$USER" "$APP_DIR"

echo "==> 6/6 daily MongoDB backup (keeps 7 days)"
cat <<'CRON' | sudo tee /etc/cron.d/kocflow-backup >/dev/null
30 2 * * * root mkdir -p /opt/kocflow/backups && docker exec kocflow-mongodb-1 mongodump --archive --gzip --db tiktoday_booking > /opt/kocflow/backups/mongo-$(date +\%F).gz && find /opt/kocflow/backups -name 'mongo-*.gz' -mtime +7 -delete
CRON

echo
echo "Done. Next: put .env in $APP_DIR (see deploy/.env.prod.example), and open TCP 80 + 443 in the OCI VCN security list."
echo "Log out/in once so the docker group applies."
