#!/bin/sh
# wtdb installer. Usage:
#   curl -fsSL https://raw.githubusercontent.com/willhackett/wtdb/main/install.sh | sh
#
# Env overrides:
#   WTDB_VERSION   version tag to install (default: latest release)
#   WTDB_BIN_DIR   install directory (default: ~/.local/bin, or /usr/local/bin if writable)
set -eu

REPO="willhackett/wtdb"

info() { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
err()  { printf '\033[1;31merror:\033[0m %s\n' "$1" >&2; exit 1; }

# --- detect platform ---------------------------------------------------------
os=$(uname -s)
case "$os" in
  Darwin) os=darwin ;;
  Linux)  os=linux ;;
  *) err "unsupported OS: $os" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- resolve version ---------------------------------------------------------
version="${WTDB_VERSION:-}"
if [ -z "$version" ]; then
  info "Resolving latest release..."
  version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -n "$version" ] || err "could not determine latest version (set WTDB_VERSION)"
fi
num="${version#v}"

# --- choose install dir ------------------------------------------------------
bin_dir="${WTDB_BIN_DIR:-}"
if [ -z "$bin_dir" ]; then
  if [ -w /usr/local/bin ] 2>/dev/null; then
    bin_dir=/usr/local/bin
  else
    bin_dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$bin_dir"

# --- download & install ------------------------------------------------------
asset="wtdb_${num}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/${version}/${asset}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

info "Downloading $asset ($version)..."
curl -fsSL "$url" -o "$tmp/wtdb.tar.gz" || err "download failed: $url"
tar -xzf "$tmp/wtdb.tar.gz" -C "$tmp"
install -m 0755 "$tmp/wtdb" "$bin_dir/wtdb"

info "Installed wtdb to $bin_dir/wtdb"

# Install the man page next to the binary's prefix (e.g. /usr/local/bin ->
# /usr/local/share/man/man1), so `man wtdb` works. Best-effort.
if [ -f "$tmp/man/wtdb.1" ]; then
  man_dir="$(dirname "$bin_dir")/share/man/man1"
  if mkdir -p "$man_dir" 2>/dev/null && cp "$tmp/man/wtdb.1" "$man_dir/wtdb.1" 2>/dev/null; then
    info "Installed man page to $man_dir/wtdb.1"
  fi
fi
case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) printf '\033[1;33mnote:\033[0m %s is not on your PATH. Add it:\n  export PATH="%s:$PATH"\n' "$bin_dir" "$bin_dir" ;;
esac

echo
echo "Next: cd into a repo, then run:"
echo "  wtdb init      # create .wtdb.json"
echo "  wtdb install   # install the git hook"
