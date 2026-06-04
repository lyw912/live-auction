#!/usr/bin/env bash
# Configure an independent ECS host as a k6 load generator.
#
# Safe scope: this script changes only the k6/client host. It does not configure
# the live-auction service node, database, Redis, Kafka, or PTS.
set -euo pipefail

NODE_EXPORTER_VERSION="${NODE_EXPORTER_VERSION:-1.9.1}"
NODE_EXPORTER_PORT="${NODE_EXPORTER_PORT:-9100}"
INSTALL_NODE_EXPORTER="${INSTALL_NODE_EXPORTER:-true}"
INSTALL_K6="${INSTALL_K6:-true}"
K6_ULIMIT_NOFILE="${K6_ULIMIT_NOFILE:-1048576}"

log() {
  printf '[bootstrap-k6-ecs] %s\n' "$*"
}

need_root() {
  if [ "$(id -u)" -ne 0 ]; then
    log "re-running through sudo"
    exec sudo -E bash "$0" "$@"
  fi
}

detect_os() {
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    OS_ID="${ID:-unknown}"
    OS_ID_LIKE="${ID_LIKE:-}"
  else
    OS_ID="unknown"
    OS_ID_LIKE=""
  fi
}

install_base_packages_deb() {
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y ca-certificates curl gnupg lsb-release jq git unzip tar gzip \
    sysstat htop procps iproute2 net-tools lsof dstat
}

install_base_packages_rpm() {
  local installer="yum"
  if command -v dnf >/dev/null 2>&1; then
    installer="dnf"
  fi
  "$installer" install -y ca-certificates curl gnupg2 jq git unzip tar gzip \
    sysstat htop procps-ng iproute net-tools lsof
  "$installer" install -y dstat || true
}

install_k6_deb() {
  local keyring="/usr/share/keyrings/k6-archive-keyring.gpg"
  curl -fsSL https://dl.k6.io/key.gpg | gpg --dearmor -o "$keyring"
  echo "deb [signed-by=${keyring}] https://dl.k6.io/deb stable main" \
    > /etc/apt/sources.list.d/k6.list
  apt-get update
  apt-get install -y k6
}

install_k6_rpm() {
  local repo_file="/etc/yum.repos.d/k6.repo"
  cat > "$repo_file" <<'REPO'
[k6]
name=k6
baseurl=https://dl.k6.io/rpm/repo
enabled=1
gpgcheck=1
gpgkey=https://dl.k6.io/key.gpg
REPO
  local installer="yum"
  if command -v dnf >/dev/null 2>&1; then
    installer="dnf"
  fi
  "$installer" install -y k6
}

install_node_exporter() {
  local arch tarball workdir url
  arch="$(uname -m)"
  case "$arch" in
    x86_64 | amd64) arch="amd64" ;;
    aarch64 | arm64) arch="arm64" ;;
    *) log "unsupported node_exporter arch: $arch"; return 1 ;;
  esac

  workdir="$(mktemp -d)"
  tarball="${workdir}/node_exporter.tar.gz"
  url="https://github.com/prometheus/node_exporter/releases/download/v${NODE_EXPORTER_VERSION}/node_exporter-${NODE_EXPORTER_VERSION}.linux-${arch}.tar.gz"

  curl -fsSL -o "$tarball" "$url"
  tar -C "$workdir" -xzf "$tarball"
  install -m 0755 "${workdir}/node_exporter-${NODE_EXPORTER_VERSION}.linux-${arch}/node_exporter" /usr/local/bin/node_exporter

  if ! id node_exporter >/dev/null 2>&1; then
    useradd --no-create-home --shell /usr/sbin/nologin node_exporter
  fi

  cat > /etc/systemd/system/node_exporter.service <<EOF
[Unit]
Description=Prometheus Node Exporter
After=network.target

[Service]
User=node_exporter
ExecStart=/usr/local/bin/node_exporter --web.listen-address=:${NODE_EXPORTER_PORT}
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable --now node_exporter
  rm -rf "$workdir"
}

configure_limits() {
  cat > /etc/security/limits.d/99-k6-loadgen.conf <<EOF
* soft nofile ${K6_ULIMIT_NOFILE}
* hard nofile ${K6_ULIMIT_NOFILE}
root soft nofile ${K6_ULIMIT_NOFILE}
root hard nofile ${K6_ULIMIT_NOFILE}
EOF

  cat > /etc/sysctl.d/99-k6-loadgen.conf <<'EOF'
net.ipv4.ip_local_port_range = 10000 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 15
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
EOF
  sysctl --system >/dev/null
}

print_summary() {
  log "done"
  echo
  echo "Versions:"
  command -v k6 >/dev/null 2>&1 && k6 version || echo "k6: not installed"
  command -v jq >/dev/null 2>&1 && jq --version || true
  command -v git >/dev/null 2>&1 && git --version || true
  echo
  echo "Host:"
  uname -a
  nproc
  free -h
  ulimit -n
  ss -s || true
  echo
  echo "node_exporter:"
  systemctl --no-pager --full status node_exporter || true
}

main() {
  need_root "$@"
  detect_os

  log "installing base packages for ${OS_ID}"
  if [[ "$OS_ID" == "ubuntu" || "$OS_ID" == "debian" || "$OS_ID_LIKE" == *"debian"* ]]; then
    install_base_packages_deb
    if [[ "$INSTALL_K6" == "true" ]] && ! command -v k6 >/dev/null 2>&1; then
      log "installing k6 from official apt repo"
      install_k6_deb
    fi
  elif [[ "$OS_ID" == "alinux" || "$OS_ID" == "almalinux" || "$OS_ID" == "rocky" || "$OS_ID" == "centos" || "$OS_ID" == "rhel" || "$OS_ID_LIKE" == *"rhel"* || "$OS_ID_LIKE" == *"fedora"* ]]; then
    install_base_packages_rpm
    if [[ "$INSTALL_K6" == "true" ]] && ! command -v k6 >/dev/null 2>&1; then
      log "installing k6 from official rpm repo"
      install_k6_rpm
    fi
  else
    log "unknown OS '${OS_ID}', installing only generic prerequisites is not implemented"
    exit 1
  fi

  log "configuring file descriptor and TCP client-side limits"
  configure_limits

  if [[ "$INSTALL_NODE_EXPORTER" == "true" ]]; then
    log "installing node_exporter ${NODE_EXPORTER_VERSION}"
    install_node_exporter
  fi

  print_summary
}

main "$@"
