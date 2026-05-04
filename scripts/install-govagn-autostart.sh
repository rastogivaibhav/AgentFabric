#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
START_SCRIPT="${REPO_ROOT}/scripts/start-govagn-stack.sh"
LOG_FILE="${REPO_ROOT}/artifacts/autostart.log"
OS="$(uname -s)"

if [[ ! -f "${START_SCRIPT}" ]]; then
  echo "Missing startup script: ${START_SCRIPT}" >&2
  exit 1
fi

mkdir -p "${REPO_ROOT}/artifacts"

install_macos() {
  local plist_dir="${HOME}/Library/LaunchAgents"
  local plist_path="${plist_dir}/io.govagn.stack.autostart.plist"
  mkdir -p "${plist_dir}"

  cat > "${plist_path}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>io.govagn.stack.autostart</string>
    <key>ProgramArguments</key>
    <array>
      <string>/bin/bash</string>
      <string>${START_SCRIPT}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${LOG_FILE}</string>
    <key>StandardErrorPath</key>
    <string>${LOG_FILE}</string>
  </dict>
</plist>
EOF

  launchctl unload "${plist_path}" >/dev/null 2>&1 || true
  launchctl load "${plist_path}"
  echo "Installed macOS LaunchAgent: ${plist_path}"
  echo "Govagn will start automatically at login."
}

install_linux_systemd() {
  local unit_dir="${HOME}/.config/systemd/user"
  local unit_path="${unit_dir}/govagn-stack.service"
  local start_escaped
  start_escaped="$(printf '%q' "${START_SCRIPT}")"
  mkdir -p "${unit_dir}"

  cat > "${unit_path}" <<EOF
[Unit]
Description=Govagn Stack Auto Start
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
WorkingDirectory=${REPO_ROOT}
ExecStart=/bin/bash -lc ${start_escaped}
RemainAfterExit=true

[Install]
WantedBy=default.target
EOF

  systemctl --user daemon-reload
  systemctl --user enable --now govagn-stack.service
  echo "Installed systemd user service: ${unit_path}"
  echo "Govagn will start automatically for this user after reboot/login."
}

install_linux_cron() {
  local start_escaped
  local log_escaped
  start_escaped="$(printf '%q' "${START_SCRIPT}")"
  log_escaped="$(printf '%q' "${LOG_FILE}")"
  local entry="@reboot /bin/bash -lc ${start_escaped} >> ${log_escaped} 2>&1"
  (crontab -l 2>/dev/null | grep -Fv "${START_SCRIPT}"; echo "${entry}") | crontab -
  echo "Installed cron @reboot entry for Govagn."
}

case "${OS}" in
  Darwin)
    install_macos
    ;;
  Linux)
    if command -v systemctl >/dev/null 2>&1; then
      install_linux_systemd
    elif command -v crontab >/dev/null 2>&1; then
      install_linux_cron
    else
      echo "No supported startup manager found (systemd/crontab)." >&2
      exit 1
    fi
    ;;
  *)
    echo "Unsupported OS: ${OS}" >&2
    exit 1
    ;;
esac
