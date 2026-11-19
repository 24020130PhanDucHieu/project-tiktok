#!/usr/bin/env bash
# One-time bootstrap for the second VM, the one that runs only zalo-bridge (Oracle Linux 9, Always Free E2.1.Micro).
# Run as the default user (opc), giving the PRIVATE IP of the backend VM:
#   bash setup-bridge-server.sh 10.0.0.12
# Safe to re-run. Runbook: docs/tach-zalo-bridge.md
set -euo pipefail

BACKEND_IP="${1:?usage: bash setup-bridge-server.sh <private IP of the backend VM>}"
APP_DIR=/opt/kocflow-bridge

echo "==> 1/5 swap (2 GB) - the VM has only 1 GB of RAM"
if [ -z "$(swapon --show --noheadings)" ]; then
  sudo fallocate -l 2G /swapfile 2>/dev/null || sudo dd if=/dev/zero of=/swapfile bs=1M count=2048
  sudo chmod 600 /swapfile
  sudo mkswap /swapfile >/dev/null
  sudo swapon /swapfile
  grep -q '/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab >/dev/null
fi
echo 'vm.swappiness=20' | sudo tee /etc/sysctl.d/99-kocflow.conf >/dev/null
sudo sysctl -q --system

echo "==> 2/5 free memory: stop services this VM does not need"
for svc in oracle-cloud-agent-updater; do sudo systemctl disable --now "$svc" 2>/dev/null || true; done

echo "==> 3/5 Docker Engine + compose plugin"
if ! command -v docker >/dev/null; then
  sudo dnf -y install dnf-plugins-core >/dev/null
  sudo dnf config-manager --add-repo https://download.docker.com/linux/rhel/docker-ce.repo
  sudo dnf -y install docker-ce docker-ce-cli containerd.io docker-compose-plugin >/dev/null
fi
sudo systemctl enable --now docker
sudo usermod -aG docker "$USER" || true
echo '{"log-driver":"json-file","log-opts":{"max-size":"10m","max-file":"3"}}' | sudo tee /etc/docker/daemon.json >/dev/null
sudo systemctl restart docker

echo "==> 4/5 firewall: port 5100 only from the backend VM ($BACKEND_IP); nothing on 80/443 here"
if systemctl is-active --quiet firewalld; then
  sudo firewall-cmd --permanent --add-rich-rule="rule family=ipv4 source address=$BACKEND_IP port port=5100 protocol=tcp accept" >/dev/null
  sudo firewall-cmd --reload >/dev/null
fi
# Oracle images also ship restrictive iptables rules; Docker's published port is filtered in the DOCKER-USER chain
sudo iptables -C INPUT -p tcp -s "$BACKEND_IP" --dport 5100 -j ACCEPT 2>/dev/null || sudo iptables -I INPUT -p tcp -s "$BACKEND_IP" --dport 5100 -j ACCEPT
sudo iptables -C DOCKER-USER ! -s "$BACKEND_IP" -p tcp --dport 5100 -j DROP 2>/dev/null || sudo iptables -I DOCKER-USER ! -s "$BACKEND_IP" -p tcp --dport 5100 -j DROP 2>/dev/null || true
command -v netfilter-persistent >/dev/null && sudo netfilter-persistent save || sudo sh -c 'iptables-save > /etc/sysconfig/iptables' 2>/dev/null || true

echo "==> 5/5 app directory"
sudo mkdir -p "$APP_DIR"
sudo chown "$USER":"$USER" "$APP_DIR"

echo
echo "Private IP of this VM (put it in $APP_DIR/.env as BRIDGE_BIND_IP):"
ip -4 -o addr show scope global | awk '{print "  " $2 ": " $4}'
echo
echo "Next: create $APP_DIR/.env (see docs/tach-zalo-bridge.md, step 5), then run the 'Deploy zalo-bridge' workflow."
echo "Log out/in once so the docker group applies."
