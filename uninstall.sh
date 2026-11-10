#!/bin/sh
# Bootstrap a verified payload so removal does not depend on the installed binary.
set -eu
umask 077
case "${1:-}" in ''|--purge) ;; *) echo 'Usage: uninstall.sh [--purge]' >&2; exit 2 ;; esac
[ "$#" -le 1 ] || { echo 'Usage: uninstall.sh [--purge]' >&2; exit 2; }
case "$(uname -s)" in
 Darwin) root="$HOME/Library/Application Support/nox-cli" ;;
 Linux) root="${XDG_DATA_HOME:-$HOME/.local/share}/nox-cli" ;;
 *) echo 'Use uninstall.ps1 on Windows.' >&2; exit 1 ;;
esac
case "$root" in /*) ;; *) echo 'The installation directory must be absolute.' >&2; exit 1 ;; esac
stage="$root/install"
[ ! -L "$root" ] && [ ! -L "$stage" ] || { echo 'Refusing a linked installation directory.' >&2; exit 1; }
mkdir -p "$stage"
bootstrap="$stage/uninstall-bootstrap.sh"
[ ! -L "$bootstrap" ] || { echo 'Refusing a linked bootstrap.' >&2; exit 1; }
cleanup() { rm -f "$bootstrap"; rmdir "$stage" 2>/dev/null || true; rmdir "$root" 2>/dev/null || true; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --connect-timeout 10 --max-time 60 \
 https://github.com/mapherez/nox-cli/releases/latest/download/install.sh -o "$bootstrap"
if [ "$#" -eq 1 ]; then sh "$bootstrap" --uninstall --purge
else sh "$bootstrap" --uninstall; fi
