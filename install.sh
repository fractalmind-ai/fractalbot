#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[fractalbot] %s\n' "$*"
}

usage() {
  cat <<'EOF'
Usage: install.sh [--systemd-user]

Options:
  --systemd-user   Install a systemd user service (Linux only).
EOF
}

systemd_user=0
while [ $# -gt 0 ]; do
  case "$1" in
    --systemd-user)
      systemd_user=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      log "Unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  log "Go is required (go command not found)."
  exit 1
fi

bin_dir="${HOME}/.local/bin"
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/fractalbot"
config_path="${config_dir}/config.yaml"
data_dir="${XDG_DATA_HOME:-$HOME/.local/share}/fractalbot"
workspace_dir="${data_dir}/workspace"

repo_root=""
cleanup=""
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -f "${script_dir}/go.mod" ]; then
  repo_root="${script_dir}"
elif command -v git >/dev/null 2>&1 && git rev-parse --show-toplevel >/dev/null 2>&1; then
  repo_root="$(git rev-parse --show-toplevel)"
else
  if ! command -v git >/dev/null 2>&1; then
    log "git is required for curl|bash installs."
    exit 1
  fi
  tmp_dir="$(mktemp -d)"
  cleanup="${tmp_dir}"
  log "Cloning fractalbot into ${tmp_dir}..."
  git clone --depth 1 https://github.com/fractalmind-ai/fractalbot.git "${tmp_dir}" >/dev/null 2>&1
  repo_root="${tmp_dir}"
fi

if [ ! -f "${repo_root}/go.mod" ]; then
  log "Repository root not found (go.mod missing)."
  exit 1
fi

mkdir -p "${bin_dir}" "${config_dir}" "${data_dir}" "${workspace_dir}"

log "Building fractalbot..."
(cd "${repo_root}" && go build -o "${bin_dir}/fractalbot" ./cmd/fractalbot)

if [ ! -f "${config_path}" ]; then
  log "Installing default config to ${config_path}..."
  cp "${repo_root}/config.example.yaml" "${config_path}"
else
  log "Config exists at ${config_path}; leaving as-is."
fi

if [ "${systemd_user}" -eq 1 ]; then
  if [ "$(uname -s)" != "Linux" ]; then
    log "--systemd-user is only supported on Linux."
    exit 1
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    log "systemctl not found; cannot install user service."
    exit 1
  fi

  unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  unit_path="${unit_dir}/fractalbot.service"
  mkdir -p "${unit_dir}"
  cat > "${unit_path}" <<EOF
[Unit]
Description=FractalBot
After=network.target

[Service]
ExecStart=${bin_dir}/fractalbot --config ${config_path}
WorkingDirectory=${data_dir}
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF

  systemctl --user daemon-reload
  systemctl --user enable --now fractalbot.service
  log "Installed systemd user service at ${unit_path}"
fi

log "Installed fractalbot to ${bin_dir}/fractalbot"
if ! command -v fractalbot >/dev/null 2>&1; then
  log "Add ${bin_dir} to your PATH to use 'fractalbot'."
fi
log "Config: ${config_path}"
log "Data: ${data_dir}"

if [ -n "${cleanup}" ]; then
  rm -rf "${cleanup}"
fi
