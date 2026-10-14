#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
go_bin="${GO:-$(command -v go || true)}"
if [[ -z "$go_bin" && -x /usr/local/go/bin/go ]]; then go_bin=/usr/local/go/bin/go; fi
if [[ -z "$go_bin" ]]; then echo 'Go is required. Install Go from https://go.dev/dl/ and retry.' >&2; exit 1; fi
cd "$root"
mkdir -p bin
"$go_bin" build -trimpath -o bin/neardrop ./cmd/neardrop
install -d "$HOME/.local/bin" "$HOME/.local/share/applications" "$HOME/.config/systemd/user" "$HOME/.local/share/icons/hicolor/scalable/apps"
install -m755 bin/neardrop "$HOME/.local/bin/neardrop"
install -m644 docs/icon.svg "$HOME/.local/share/icons/hicolor/scalable/apps/neardrop.svg"
cat > "$HOME/.config/systemd/user/neardrop.service" <<'EOF'
[Unit]
Description=NearDrop local photo receiver
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%h/.local/bin/neardrop
Restart=on-failure
RestartSec=5
UMask=0077
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=default.target
EOF
cat > "$HOME/.local/share/applications/neardrop.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=NearDrop
Comment=Receive photos from your iPhone
Exec=xdg-open http://127.0.0.1:8788
Icon=neardrop
Terminal=false
Categories=Utility;Network;
EOF
systemctl --user daemon-reload
systemctl --user enable --now neardrop.service
echo 'NearDrop is installed and starts when you log in.'
echo 'Open http://127.0.0.1:8788 to pair your iPhone.'
echo 'Incoming files: ~/Pictures/NearDrop'
