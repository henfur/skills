#!/usr/bin/env bash
# Builds lacarta, seeds a scratch home with real projects through the CLI and hooks, and serves
# the console on $PORT. Used by Playwright (playwright.config.js); never touches ~/.lacarta.
set -euo pipefail
cd "$(dirname "$0")/../../.."          # lacarta-go/
RUN="$PWD/web/app/tests/.run"
rm -rf "$RUN" && mkdir -p "$RUN/claude/projects/x" "$RUN/task" "$RUN/done-task"
export LACARTA_HOME="$RUN/home" CLAUDE_CONFIG_DIR="$RUN/claude" LACARTA_NO_DAEMON=1
unset LACARTA_PROJECT LACARTA_SESSION LACARTA_AGENT   # from the session running the tests
go build -o "$RUN/lacarta" ./cmd/lacarta
L="$RUN/lacarta"
mkdir -p "$LACARTA_HOME" && echo '{"remote_control": false}' > "$LACARTA_HOME/settings.json"

# A single-agent task with a transcript and a question.
T="$CLAUDE_CONFIG_DIR/projects/x/s1.jsonl"
cat > "$T" <<'JSONL'
{"type":"user","message":{"content":"Fix the login bug"},"timestamp":"2026-11-12T01:00:00Z"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Looking at **auth.py**."},{"type":"tool_use","name":"Bash","input":{"command":"grep -rn login","description":"Search login code"}},{"type":"tool_use","name":"Read","input":{"file_path":"auth.py"}}]},"timestamp":"2026-11-12T01:00:05Z"}
JSONL
echo "{\"session_id\":\"s1\",\"cwd\":\"$RUN/task\",\"transcript_path\":\"$T\"}" | CLAUDE_ENV_FILE="$RUN/env1" "$L" hook session-start > /dev/null
echo '{"session_id":"s1","prompt":"Fix the login bug"}' | "$L" hook prompt
TASK=$(sed -n 's/^export LACARTA_PROJECT=//p' "$RUN/env1")
"$L" --project "$TASK" ask --title "Which fix?" --option "Patch|Small change, quick to review" --option "Rewrite|Cleaner, takes a day" > /dev/null

# An ended task, which can be archived.
echo "{\"session_id\":\"s3\",\"cwd\":\"$RUN/done-task\"}" | CLAUDE_ENV_FILE="$RUN/env3" "$L" hook session-start > /dev/null
echo '{"session_id":"s3","reason":"exit"}' | "$L" hook session-end
DONE=$(sed -n 's/^export LACARTA_PROJECT=//p' "$RUN/env3")

# A software project with a connected session, a diagram in a spec and a blocking finding.
"$L" --root "$RUN/shop" init --name "Shop" --summary "An online shop" > /dev/null
printf '# Architecture\n\n```mermaid\nflowchart LR\n  A[Browser] --> B[API]\n```\n' > "$RUN/shop/.lacarta/specs/architecture.md"
echo "{\"session_id\":\"s2\",\"cwd\":\"$RUN/shop\"}" | "$L" hook session-start > /dev/null
"$L" --root "$RUN/shop" finding add --phase spec --review specification --severity high --title "No auth model" \
  --scenario "Anyone can read every order" --recommendation "Specify sessions" > /dev/null
SHOP=$(sed -n 's/.*"\(shop-[0-9a-f]*\)".*/\1/p' "$LACARTA_HOME/hub.json" | head -1)

# A security audit waiting for the auditor's review (needs the security-audit skill, node and bubblewrap).
AUDIT=""
mkdir -p "$RUN/target/src" && echo "// api" > "$RUN/target/src/api.js"
if "$L" audit preflight > "$RUN/preflight.txt" 2>&1; then
  AUDIT=$("$L" audit init --target "$RUN/target" --name "Orders API" | sed -n 's/^Security audit \(audit-[a-z0-9-]*\) of.*/\1/p')
  A() { "$L" --project "$AUDIT" "$@"; }
  A audit scope --profile quick --budget 20 > /dev/null
  A answer Q-1 --choice Approve > /dev/null
  RD="$LACARTA_HOME/audits/$AUDIT/run-1"
  SKILL=$(sed -n 's/^ok   security-audit skill *\(\/[^ ]*\) .*/\1/p' "$RUN/preflight.txt")
  mkdir -p "$RD" && echo "# Architecture" > "$RD/architecture.md" && echo '{"run_status":"in_progress"}' > "$RD/run-metadata.json"
  node "$PWD/web/app/tests/audit-fixture.cjs" "$SKILL/validate-coverage-ledger.cjs" "$RD" plan.json
  mv "$RD/plan.json" "$RD/coverage-ledger.json"
  A audit plan submit > /dev/null
  A answer Q-2 --choice Approve > /dev/null
  node "$PWD/web/app/tests/audit-fixture.cjs" "$SKILL/validate-coverage-ledger.cjs" "$RD" coverage-ledger.json
  A audit findings submit > /dev/null
  A audit summary --text "One high finding: any user can update any order." > /dev/null
fi

printf '{"task":"%s","done":"%s","shop":"%s","audit":"%s","lacarta":"%s","home":"%s","shopRoot":"%s"}\n' \
  "$TASK" "$DONE" "$SHOP" "$AUDIT" "$L" "$LACARTA_HOME" "$RUN/shop" > "$RUN/ids.json"
unset LACARTA_NO_DAEMON
exec "$L" serve --port "${PORT:-4799}"
