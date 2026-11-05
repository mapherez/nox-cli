#!/bin/sh
# Download/verify the official payload; the native executable owns activation.
set -eu
umask 077

case "$(uname -s)" in
  Darwin) platform=darwin; root="$HOME/Library/Application Support/nox-cli" ;;
  Linux) platform=linux; root="${XDG_DATA_HOME:-$HOME/.local/share}/nox-cli" ;;
  *) echo 'This installer supports macOS and Linux. Use install.ps1 on Windows.' >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) architecture=arm64 ;;
  x86_64|amd64) architecture=amd64 ;;
  *) echo 'Unsupported CPU architecture.' >&2; exit 1 ;;
esac
case "$root" in /*) ;; *) echo 'The installation directory must be absolute.' >&2; exit 1 ;; esac
for tool in curl tar awk; do command -v "$tool" >/dev/null 2>&1 || { echo "Required system utility is unavailable: $tool" >&2; exit 1; }; done
if command -v sha256sum >/dev/null 2>&1; then hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then hash_tool=shasum
else echo 'A SHA-256 utility is required.' >&2; exit 1; fi

stage="$root/install"
[ ! -L "$root" ] && [ ! -L "$stage" ] || { echo 'Refusing a linked installation directory.' >&2; exit 1; }
mkdir -p "$stage"
chmod 700 "$root" "$stage"
lock="$stage/installer.lock"
[ ! -L "$lock" ] || { echo 'Refusing a linked installer lock.' >&2; exit 1; }
if [ -d "$lock" ]; then
  previous_pid=$(cat "$lock/pid" 2>/dev/null || true)
  if [ -z "$previous_pid" ]; then
    # A process can stop between creating its directory and writing the PID.
    sleep 1
    previous_pid=$(cat "$lock/pid" 2>/dev/null || true)
  fi
  case "$previous_pid" in
    '') ;;
    *[!0-9]*) echo 'An existing installer lock is invalid.' >&2; exit 1 ;;
    *) if kill -0 "$previous_pid" 2>/dev/null; then echo 'Another NoX CLI installer is running.' >&2; exit 1; fi ;;
  esac
  rm -f "$lock/pid"; rmdir "$lock"
fi
mkdir "$lock"
printf '%s\n' "$$" > "$lock/pid"
cleanup() {
  rm -f "$stage/payload.tar.gz" "$stage/checksums.txt" "$stage/nox"
  for item in "$stage"/activate-*; do [ ! -e "$item" ] || rm -f "$item"; done
  rm -f "$lock/pid"
  rmdir "$lock" 2>/dev/null || true
  rmdir "$stage" 2>/dev/null || true
  rmdir "$root" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Clear only the known files owned by an interrupted download/activation.
rm -f "$stage/payload.tar.gz" "$stage/checksums.txt" "$stage/nox"
for item in "$stage"/activate-*; do [ ! -e "$item" ] || rm -f "$item"; done

asset="nox-cli_${platform}_${architecture}.tar.gz"
release='https://github.com/mapherez/nox-cli/releases/latest/download'
if ! curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$release/$asset" -o "$stage/payload.tar.gz" ||
   ! curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' "$release/checksums.txt" -o "$stage/checksums.txt"; then
  echo 'Unable to download the CLI release. Check connectivity and whether a release has been published.' >&2; exit 1
fi
if ! expected=$(awk -v asset="$asset" '$2 == asset || $2 == "*" asset { value=$1; count++ } END { if(count != 1) exit 1; print value }' "$stage/checksums.txt"); then
  echo 'The release checksum entry is missing or duplicated.' >&2; exit 1
fi
case "$expected" in ''|*[!a-fA-F0-9]*) echo 'Invalid release checksum.' >&2; exit 1 ;; esac
[ "${#expected}" -eq 64 ] || { echo 'Invalid release checksum.' >&2; exit 1; }
if [ "$hash_tool" = sha256sum ]; then actual=$(sha256sum "$stage/payload.tar.gz" | awk '{print $1}')
else actual=$(shasum -a 256 "$stage/payload.tar.gz" | awk '{print $1}'); fi
[ "$actual" = "$expected" ] || { echo 'Release checksum verification failed.' >&2; exit 1; }
# Accept exactly one regular executable name, never archive paths or links.
[ "$(tar -tzf "$stage/payload.tar.gz")" = nox ] || { echo 'Unexpected release archive contents.' >&2; exit 1; }
case "$(tar -tvzf "$stage/payload.tar.gz")" in -*) ;; *) echo 'The release executable must be a regular file.' >&2; exit 1 ;; esac
tar -xzf "$stage/payload.tar.gz" -C "$stage"
[ -f "$stage/nox" ] && [ ! -L "$stage/nox" ] || { echo 'The release executable is invalid.' >&2; exit 1; }
chmod 700 "$stage/nox"
"$stage/nox" --complete-install
