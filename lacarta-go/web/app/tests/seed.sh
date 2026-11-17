#!/usr/bin/env bash
# Builds lacarta, seeds a scratch home with real projects through the CLI and hooks, and serves
# the console on $PORT. Used by Playwright (playwright.config.js); never touches ~/.lacarta.
set -euo pipefail
cd "$(dirname "$0")/../../.."          # lacarta-go/
RUN="$PWD/web/app/tests/.run"
rm -rf "$RUN" && mkdir -p "$RUN/claude/projects/x"
# The tasks' folders sit outside this repository: a task's config.json goes to its git root.
WORK=$(mktemp -d "${TMPDIR:-/tmp}/lacarta-seed.XXXXXX")
mkdir -p "$WORK/done-task" "$WORK/notes/.lacarta"
git init -q "$WORK/login-fix"
echo '{"name": "Release notes"}' > "$WORK/notes/.lacarta/config.json"
export LACARTA_HOME="$RUN/home" CLAUDE_CONFIG_DIR="$RUN/claude" LACARTA_NO_DAEMON=1
unset LACARTA_PROJECT LACARTA_SESSION LACARTA_AGENT   # from the session running the tests
go build -o "$RUN/lacarta" ./cmd/lacarta
L="$RUN/lacarta"
mkdir -p "$LACARTA_HOME" && echo '{"//": "seeded", "remote_control": false}' > "$LACARTA_HOME/config.json"

# A single-agent task with a transcript and a question.
T="$CLAUDE_CONFIG_DIR/projects/x/s1.jsonl"
cat > "$T" <<'JSONL'
{"type":"user","message":{"content":"Fix the login bug"},"timestamp":"2026-11-12T01:00:00Z"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Looking at **auth.py**."},{"type":"tool_use","name":"Bash","input":{"command":"grep -rn login","description":"Search login code"}},{"type":"tool_use","name":"Read","input":{"file_path":"auth.py"}}]},"timestamp":"2026-11-12T01:00:05Z"}
JSONL
echo "{\"session_id\":\"s1\",\"cwd\":\"$WORK/login-fix\",\"transcript_path\":\"$T\"}" | CLAUDE_ENV_FILE="$RUN/env1" "$L" hook session-start > /dev/null
echo '{"session_id":"s1","prompt":"Fix the login bug"}' | "$L" hook prompt
TASK=$(sed -n 's/^export LACARTA_PROJECT=//p' "$RUN/env1")
"$L" --project "$TASK" ask --title "Which fix?" --option "Patch|Small change, quick to review" --option "Rewrite|Cleaner, takes a day" > /dev/null

# An ended task, which can be archived.
echo "{\"session_id\":\"s3\",\"cwd\":\"$WORK/done-task\"}" | CLAUDE_ENV_FILE="$RUN/env3" "$L" hook session-start > /dev/null
echo '{"session_id":"s3","reason":"exit"}' | "$L" hook session-end
DONE=$(sed -n 's/^export LACARTA_PROJECT=//p' "$RUN/env3")
# An ended task that did something: listed under Inactive (the one above is an empty session).
echo "{\"session_id\":\"s4\",\"cwd\":\"$WORK/notes\"}" | CLAUDE_ENV_FILE="$RUN/env4" "$L" hook session-start > /dev/null
echo '{"session_id":"s4","prompt":"Write the release notes"}' | "$L" hook prompt
echo '{"session_id":"s4","reason":"exit"}' | "$L" hook session-end

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

# A review desk with a review waiting for the user. GitLab is the Go tests' fake glab: the cli test
# binary acts as glab when it runs under that name.
mkdir -p "$RUN/bin" "$WORK/xdg/glab-desk-write" "$WORK/xdg/glab-desk-read" "$RUN/desk/.lacarta"
go test -c -o "$RUN/bin/glab" ./internal/cli
export PATH="$RUN/bin:$PATH" XDG_CONFIG_HOME="$WORK/xdg" LACARTA_FAKE_GLAB="$RUN/gitlab.json"
cat > "$LACARTA_FAKE_GLAB" <<'JSON'
{"host": "gitlab.example.com", "next_id": 100,
 "identities": {"glab-desk-write": {"user": "review-bot", "scopes": ["api"]}, "glab-desk-read": {"user": "reader", "scopes": ["read_api"]}},
 "mrs": [{"project_id": 3, "project": "acme/billing", "iid": 7, "title": "Export downloads", "state": "opened", "author": "dev",
   "sha": "aaa1111111111111", "base_sha": "base000000000000", "source_branch": "export", "target_branch": "main", "labels": [],
   "pipeline": "success", "updated_at": "2026-11-17T10:00:00Z",
   "changes": [{"old_path": "src/export.go", "new_path": "src/export.go", "diff": "@@ -1 +1,3 @@\n+func Download(id string) {\n+}\n"}]}]}
JSON
cat > "$RUN/desk/.lacarta/config.json" <<JSON
{"name": "Platform desk", "desk": {"forge": {"type": "gitlab", "host": "gitlab.example.com", "identity": "review-bot",
  "write_config": "$WORK/xdg/glab-desk-write", "read_config": "$WORK/xdg/glab-desk-read"},
  "targets": [{"group": "acme"}], "profiles": {"code": {"mode": "shadow"}}}}
JSON
DESK=$("$L" desk init --folder "$RUN/desk" | sed -n 's/^Review desk \([a-z0-9-]*\) in .*/\1/p')
D() { "$L" --project "$DESK" "$@"; }
D desk next --timeout 10 > /dev/null
cat > "$RUN/code.json" <<'JSON'
{"profile": "code", "mr": {"project": "acme/billing", "iid": 7, "head_sha": "aaa1111111111111"}, "status": "complete",
 "verdict": "comments", "summary": "Adds the export download; one missing test.", "notes": [], "injection_attempts": [],
 "findings": [{"id": "F-1", "fingerprint": "tests:src/export.go:Download", "class": "tests", "severity": "medium",
   "title": "Download has no test", "file": "src/export.go", "line": 1, "table": "The new download path is untested.",
   "body": "Nothing covers Download.", "fix": "Add a test for a missing id.", "confidence": "confirmed"}]}
JSON
D desk submit "$RUN/code.json" --item 'acme/billing!7' --profile code > /dev/null
D desk done 'acme/billing!7' > /dev/null

printf '{"task":"%s","done":"%s","shop":"%s","audit":"%s","desk":"%s","lacarta":"%s","home":"%s","shopRoot":"%s","gitlab":"%s"}\n' \
  "$TASK" "$DONE" "$SHOP" "$AUDIT" "$DESK" "$L" "$LACARTA_HOME" "$RUN/shop" "$LACARTA_FAKE_GLAB" > "$RUN/ids.json"
unset LACARTA_NO_DAEMON
exec "$L" serve --port "${PORT:-4799}"
