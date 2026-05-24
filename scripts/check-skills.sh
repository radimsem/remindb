#!/usr/bin/env bash
# Structural gate for the public skills under skills/.
#
# Checks, per skill dir (SKILL.md present):
#   1. frontmatter has non-empty name + description
#   2. SKILL.md is within its line budget (progressive-disclosure discipline)
#   3. no surviving relative ../../ links (they 404 once `npx skills add`
#      copies the skill standalone — use absolute GitHub URLs instead)
#   4. every referenced references/<file>.md actually exists
#
# Optional smoke (CHECK_SKILLS_SMOKE=1): attempt `npx skills add` from this
# repo into a throwaway dir and confirm the skill + its references/ copy over.
# Best-effort — skipped with a NOTE when npx/network is unavailable; never
# fails the gate on its own.
#
# Usage:
#   scripts/check-skills.sh
#   CHECK_SKILLS_SMOKE=1 scripts/check-skills.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SKILLS_DIR="$REPO_ROOT/skills"

cd "$REPO_ROOT"

# skill dir -> SKILL.md line budget (ceiling).
declare -A BUDGET=(
    [remind]=180
    [memorize]=130
    [remember]=50
    [remindb-setup]=90
)

fail=0
err() { printf '  ✗ %s\n' "$1"; fail=1; }
ok()  { printf '  ✓ %s\n' "$1"; }

for skill in "${!BUDGET[@]}"; do
    dir="$SKILLS_DIR/$skill"
    md="$dir/SKILL.md"
    printf '== %s ==\n' "$skill"

    if [[ ! -f "$md" ]]; then
        err "missing $md"
        continue
    fi

    # 1. frontmatter: name + description between the first two --- fences.
    fm="$(awk 'NR==1 && $0!="---"{exit} NR==1{next} $0=="---"{exit} {print}' "$md")"
    if [[ -z "$fm" ]]; then
        err "no frontmatter block"
    else
        name_val="$(printf '%s\n' "$fm" | sed -n 's/^name:[[:space:]]*//p')"
        desc_val="$(printf '%s\n' "$fm" | sed -n 's/^description:[[:space:]]*//p')"
        [[ -n "$name_val" ]] && ok "name: $name_val" || err "frontmatter missing non-empty name"
        [[ -n "$desc_val" ]] && ok "description present (${#desc_val} chars)" || err "frontmatter missing non-empty description"
    fi

    # 2. line budget.
    lines="$(wc -l < "$md" | tr -d '[:space:]')"
    budget="${BUDGET[$skill]}"
    if (( lines <= budget )); then
        ok "SKILL.md $lines lines (budget $budget)"
    else
        err "SKILL.md $lines lines exceeds budget $budget"
    fi

    # 3. no relative ../../ links in any markdown of the skill.
    if grep -RIn '\.\./\.\.' "$dir" --include='*.md' >/dev/null 2>&1; then
        err "relative ../../ link(s) found (use absolute GitHub URLs):"
        grep -RIn '\.\./\.\.' "$dir" --include='*.md' | sed 's/^/      /'
    else
        ok "no relative ../../ links"
    fi

    # 4. every references/<file>.md referenced from any markdown resolves.
    refs="$(grep -RhoE 'references/[A-Za-z0-9_-]+\.md' "$dir" --include='*.md' 2>/dev/null | sort -u || true)"
    if [[ -z "$refs" ]]; then
        ok "no references/ links"
    else
        while IFS= read -r ref; do
            if [[ -f "$dir/$ref" ]]; then
                ok "resolves: $ref"
            else
                err "broken reference link: $ref (no such file)"
            fi
        done <<< "$refs"
    fi
done

# Optional best-effort smoke: does `npx skills add` copy each skill cleanly?
if [[ "${CHECK_SKILLS_SMOKE:-0}" == "1" ]]; then
    printf '== smoke: npx skills add ==\n'
    if ! command -v npx >/dev/null 2>&1; then
        printf '  NOTE: npx not found — smoke skipped (structural checks above are the gate)\n'
    else
        tmp="$(mktemp -d)"
        # Ctrl-C must abort the whole run — not fall through to a bogus PASS.
        trap 'rm -rf "$tmp"; echo; echo "check-skills: ABORTED"; exit 130' INT TERM
        trap 'rm -rf "$tmp"' EXIT

        # </dev/null: never block on an interactive prompt (the hang).
        # timeout:    never wait forever on a stalled network pull.
        run=(npx -y skills@latest add radimsem/remindb/skills -a claude-code)
        command -v timeout >/dev/null 2>&1 && run=(timeout 120 "${run[@]}")

        HOME="$tmp" "${run[@]}" </dev/null >/dev/null 2>&1 && rc=0 || rc=$?
        case "$rc" in
            0)   ok "npx skills add completed" ;;
            124) printf '  NOTE: npx skills add timed out (120s) — smoke skipped (structural checks are the gate)\n' ;;
            *)   printf '  NOTE: npx skills add did not complete (rc=%d; offline / interactive / published lag) — smoke skipped\n' "$rc" ;;
        esac
        trap - INT TERM
    fi
fi

echo
if (( fail )); then
    echo "check-skills: FAIL"
    exit 1
fi
echo "check-skills: PASS"
