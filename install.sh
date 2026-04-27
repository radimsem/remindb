#!/usr/bin/env bash
set -euo pipefail

repo="radimsem/remindb"
prefix="$HOME/.local"

usage() {
	cat <<EOF
Usage: $0 [--prefix PATH]

Download and install the latest remindb release from GitHub.

Options:
  --prefix PATH   Install root; binary is placed at PATH/bin/remindb.
                  Default: \$HOME/.local
  -h, --help      Show this help.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
		--prefix)
			[ $# -ge 2 ] || { echo "error: --prefix requires a value" >&2; exit 1; }
			prefix="$2"
			shift 2
			;;
		--prefix=*)
			prefix="${1#--prefix=}"
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "error: unknown argument: $1" >&2
			usage >&2
			exit 1
			;;
	esac
done

for cmd in curl tar awk sed; do
	command -v "$cmd" >/dev/null 2>&1 || { echo "error: '$cmd' is required" >&2; exit 1; }
done

if command -v sha256sum >/dev/null 2>&1; then
	sha256() { sha256sum "$@"; }
elif command -v shasum >/dev/null 2>&1; then
	sha256() { shasum -a 256 "$@"; }
else
	echo "error: neither sha256sum nor shasum is available" >&2
	exit 1
fi

os_raw="$(uname -s)"
case "$os_raw" in
	Linux)  os="Linux" ;;
	Darwin) os="Darwin" ;;
	*) echo "error: unsupported OS: $os_raw" >&2; exit 1 ;;
esac

arch_raw="$(uname -m)"
case "$arch_raw" in
	x86_64|amd64)  arch="x86_64" ;;
	aarch64|arm64) arch="arm64" ;;
	*) echo "error: unsupported architecture: $arch_raw" >&2; exit 1 ;;
esac

echo "Resolving latest release for $repo..."
api_url="https://api.github.com/repos/$repo/releases/latest"
tag="$(curl -fsSL "$api_url" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
if [ -z "$tag" ]; then
	echo "error: failed to resolve latest release tag" >&2
	exit 1
fi

version="${tag#v}"
archive="remindb_${version}_${os}_${arch}.tar.gz"
download_url="https://github.com/$repo/releases/download/$tag/$archive"
checksums_url="https://github.com/$repo/releases/download/$tag/checksums.txt"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "Downloading $archive..."
curl -fsSL -o "$tmpdir/$archive" "$download_url"

echo "Verifying checksum..."
curl -fsSL -o "$tmpdir/checksums.txt" "$checksums_url"

expected="$(awk -v f="$archive" '$2 == f {print $1}' "$tmpdir/checksums.txt")"
if [ -z "$expected" ]; then
	echo "error: $archive not listed in checksums.txt" >&2
	exit 1
fi

actual="$(sha256 "$tmpdir/$archive" | awk '{print $1}')"
if [ "$expected" != "$actual" ]; then
	echo "error: checksum mismatch for $archive" >&2
	echo "  expected: $expected" >&2
	echo "  actual:   $actual" >&2
	exit 1
fi

bindir="$prefix/bin"
mkdir -p "$bindir"

echo "Installing to $bindir/remindb..."
tar -xzf "$tmpdir/$archive" -C "$tmpdir"
mv "$tmpdir/remindb" "$bindir/remindb"
chmod +x "$bindir/remindb"

echo "Installed: $bindir/remindb ($tag)"

case ":$PATH:" in
	*":$bindir:"*) ;;
	*)
		echo
		echo "Note: $bindir is not on your PATH. Add it to your shell config:"
		echo "  export PATH=\"$bindir:\$PATH\""
		;;
esac
