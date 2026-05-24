#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
	echo "usage: $0 vX.Y.Z" >&2
	echo "       $0 vX.Y.Z-rc.N    (pre-release form is also accepted)" >&2
	exit 1
fi

new="$1"
if [[ ! "$new" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.+-]+)?$ ]]; then
	echo "error: version must match vX.Y.Z (optionally with -<pre> or +<meta> suffix)" >&2
	exit 1
fi

unprefixed="${new#v}"

cd "$(dirname "$0")/.."

json_files=(
	plugins/claude-code/.claude-plugin/plugin.json
	plugins/codex/remindb/.codex-plugin/plugin.json
	plugins/gemini-cli/gemini-extension.json
	plugins/openclaw/openclaw.plugin.json
	plugins/opencode/package.json
	plugins/openclaw/package.json
)

yaml_files=(
	plugins/hermes-agent/memory/remindb/plugin.yaml
)

for f in "${json_files[@]}"; do
	if [ ! -f "$f" ]; then
		echo "error: $f not found" >&2
		exit 1
	fi
	sed -i 's/^\([[:space:]]*"version":[[:space:]]*"\)[^"]*"/\1'"$unprefixed"'"/' "$f"
	echo "updated: $f -> $unprefixed"
done

for f in "${yaml_files[@]}"; do
	if [ ! -f "$f" ]; then
		echo "error: $f not found" >&2
		exit 1
	fi
	sed -i 's/^\(version:[[:space:]]*\).*/\1'"$unprefixed"'/' "$f"
	echo "updated: $f -> $unprefixed"
done
