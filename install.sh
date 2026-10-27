#!/bin/sh
# installs aivault on mac or linux:
#   curl -fsSL https://raw.githubusercontent.com/jxherc/aivault/main/install.sh | sh
# env: AIVAULT_VERSION (default latest), AIVAULT_DIR (default ~/.local/bin),
# AIVAULT_NO_INIT=1 to skip the setup question, AIVAULT_BASE_URL to download
# from somewhere other than github releases
set -eu

repo=jxherc/aivault
dir=${AIVAULT_DIR:-$HOME/.local/bin}
ver=${AIVAULT_VERSION:-latest}

say() { printf '%s\n' "$*"; }
die() { printf 'aivault install: %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) os=macos ;;
  Linux) os=linux ;;
  *) die "this script is for mac and linux. on windows: irm https://raw.githubusercontent.com/$repo/main/install.ps1 | iex" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=x64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "no build for $(uname -m). try: go install github.com/$repo/cmd/aivault@latest" ;;
esac

if command -v curl >/dev/null 2>&1; then
  get() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  get() { wget -qO "$2" "$1"; }
else
  die "needs curl or wget"
fi

if [ -n "${AIVAULT_BASE_URL:-}" ]; then
  base=$AIVAULT_BASE_URL # a mirror, or a local build for testing
elif [ "$ver" = latest ]; then
  base="https://github.com/$repo/releases/latest/download"
else
  base="https://github.com/$repo/releases/download/v${ver#v}"
fi
[ "$os" = macos ] && { [ "$arch" = arm64 ] && arch=apple-silicon || arch=intel; }
file="aivault-$os-$arch.tar.gz"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "downloading $file"
get "$base/$file" "$tmp/$file" || die "download failed. is there a release yet? see https://github.com/$repo/releases"
get "$base/checksums.txt" "$tmp/checksums.txt" || die "couldn't get checksums.txt"

want=$(grep " $file\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$file" | cut -d' ' -f1)
else
  got=$(shasum -a 256 "$tmp/$file" | cut -d' ' -f1)
fi
[ -n "$want" ] && [ "$want" = "$got" ] || die "checksum mismatch, not installing"

tar -xzf "$tmp/$file" -C "$tmp" aivault
mkdir -p "$dir"
# move, don't overwrite in place: a running mcp server keeps the old file open
mv -f "$tmp/aivault" "$dir/aivault.new"
chmod 755 "$dir/aivault.new"
mv -f "$dir/aivault.new" "$dir/aivault"
say "installed $("$dir/aivault" version) to $dir/aivault"

case ":$PATH:" in
  *":$dir:"*) ;;
  *)
    say ""
    say "$dir isn't on your PATH. add this to your shell profile (~/.zshrc or ~/.bashrc):"
    say "  export PATH=\"$dir:\$PATH\""
    ;;
esac

if [ -f "$HOME/.aivault/config.json" ]; then
  say ""
  say "already set up. \`aivault doctor\` checks everything still works."
elif [ -z "${AIVAULT_NO_INIT:-}" ] && [ -r /dev/tty ] && [ -t 1 ]; then
  say ""
  printf 'set up your vault now? [Y/n] '
  read -r ans </dev/tty || ans=n
  case "$ans" in
    n* | N*) say "later: aivault init" ;;
    *) exec "$dir/aivault" init </dev/tty ;;
  esac
else
  say "next: aivault init"
fi
