#!/usr/bin/env bash
# Installs deploy-check and deploy-tui from this package: copies the
# prebuilt binaries and config template to their runtime locations,
# installs the systemd units, and sets up the sudoers rule deploy-tui's
# "trigger now" feature depends on.
#
# Run this from inside the extracted package directory (alongside bin/,
# config/, systemd/), produced by build_package.sh on the dev machine:
#   sudo ./install.sh
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
	echo "must be run as root, e.g.: sudo $0" >&2
	exit 1
fi

PKG_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_USER="${SERVICE_USER:-${SUDO_USER:-$(logname)}}"
CONFIG_DIR=/etc/deployment-manager
CONFIG_FILE="$CONFIG_DIR/config.json"
BIN_DIR=/usr/local/bin
UNIT_DIR=/etc/systemd/system
SUDOERS_FILE=/etc/sudoers.d/deploy-tui

for f in bin/deploy-check bin/deploy-tui config/config.json systemd/deploy-check.service systemd/deploy-check.timer; do
	if [[ ! -f "$PKG_DIR/$f" ]]; then
		echo "missing expected package file: $f (run this from the directory extracted from the deploy-manager tarball)" >&2
		exit 1
	fi
done

echo "Installing deploy-manager for user: $SERVICE_USER"

install -d -m 755 "$BIN_DIR"
install -m 755 "$PKG_DIR/bin/deploy-check" "$BIN_DIR/deploy-check"
install -m 755 "$PKG_DIR/bin/deploy-tui" "$BIN_DIR/deploy-tui"

install -d -m 755 "$CONFIG_DIR"
if [[ -f "$CONFIG_FILE" ]]; then
	echo "Config already exists at $CONFIG_FILE, leaving it untouched."
else
	install -m 644 "$PKG_DIR/config/config.json" "$CONFIG_FILE"
	echo "Installed template config to $CONFIG_FILE -- edit it before relying on the timer."
fi

install -d -m 755 "$UNIT_DIR"
sed "s/User=__SERVICE_USER__/User=$SERVICE_USER/" "$PKG_DIR/systemd/deploy-check.service" >"$UNIT_DIR/deploy-check.service"
install -m 644 "$PKG_DIR/systemd/deploy-check.timer" "$UNIT_DIR/deploy-check.timer"

systemctl daemon-reload
systemctl enable --now deploy-check.timer

# deploy-tui shells out to `sudo systemctl start deploy-check.service` for its
# "trigger now" action. Without a passwordless rule scoped to exactly this
# command, sudo would prompt on deploy-tui's controlling TTY and corrupt the
# bubbletea UI, so this rule is required for that feature to work at all.
SYSTEMCTL_BIN="$(command -v systemctl)"
TMP_SUDOERS="$(mktemp)"
echo "$SERVICE_USER ALL=(root) NOPASSWD: $SYSTEMCTL_BIN start deploy-check.service" >"$TMP_SUDOERS"
if visudo -cf "$TMP_SUDOERS"; then
	install -m 440 "$TMP_SUDOERS" "$SUDOERS_FILE"
	echo "Installed sudoers rule at $SUDOERS_FILE"
else
	echo "generated sudoers rule failed validation, not installed" >&2
	rm -f "$TMP_SUDOERS"
	exit 1
fi
rm -f "$TMP_SUDOERS"

cat <<EOF

Done.
  - binaries:    $BIN_DIR/deploy-check, $BIN_DIR/deploy-tui
  - config:      $CONFIG_FILE
  - units:       $UNIT_DIR/deploy-check.service, $UNIT_DIR/deploy-check.timer (enabled)
  - sudoers:     $SUDOERS_FILE (lets $SERVICE_USER run deploy-tui's "trigger now" without a password prompt)
EOF
