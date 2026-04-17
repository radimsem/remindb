#!/usr/bin/env bash
set -euo pipefail

prefix="$HOME/.local"

usage() {
	cat <<EOF
Usage: $0 [--prefix PATH]

Build and install the remindb binary.

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

if ! command -v go >/dev/null 2>&1; then
	echo "error: 'go' is not installed or not on PATH" >&2
	exit 1
fi

cd "$(dirname "$0")"

bindir="$prefix/bin"
mkdir -p "$bindir"

echo "Building remindb -> $bindir/remindb"
go build -o "$bindir/remindb" ./cmd/remindb

echo "Installed: $bindir/remindb"

case ":$PATH:" in
	*":$bindir:"*) ;;
	*)
		echo
		echo "Note: $bindir is not on your PATH. Add it to your shell config:"
		echo "  export PATH=\"$bindir:\$PATH\""
		;;
esac
