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
# Sessions stay out of the console until asked: the agent runs `track` with the session's variables.
track() { (cd "$1" && LACARTA_SESSION="$2" LACARTA_TRANSCRIPT="${3:-}" "$L" track | sed -n 's/.*(\(t-[0-9a-f]*\)).*/\1/p' | head -1); }
echo "{\"session_id\":\"s1\",\"cwd\":\"$WORK/login-fix\",\"transcript_path\":\"$T\"}" | CLAUDE_ENV_FILE="$RUN/env1" "$L" hook session-start > /dev/null
TASK=$(track "$WORK/login-fix" s1 "$T")
echo '{"session_id":"s1","prompt":"Fix the login bug"}' | "$L" hook prompt
"$L" --project "$TASK" ask --title "Which fix?" --option "Patch|Small change, quick to review" --option "Rewrite|Cleaner, takes a day" > /dev/null

# An ended task, which can be archived.
echo "{\"session_id\":\"s3\",\"cwd\":\"$WORK/done-task\"}" | CLAUDE_ENV_FILE="$RUN/env3" "$L" hook session-start > /dev/null
DONE=$(track "$WORK/done-task" s3)
echo '{"session_id":"s3","reason":"exit"}' | "$L" hook session-end
# An ended task that did something: listed under Inactive (the one above is an empty session).
# Its folder was tracked from a terminal beforehand, so its session joins on its own.
(cd "$WORK/notes" && "$L" track > /dev/null)
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

# An incident case: a timeline, an investigation, an action and a situation report waiting for the lead.
CASE=$("$L" incident init --folder "$RUN/case" --name "Phished mailbox" --severity medium --affected mail | sed -n 's/^Case \([a-z0-9-]*\) opened in .*/\1/p')
C() { "$L" --project "$CASE" "$@"; }
C incident evidence add --title "Mail gateway log" --kind log --source mx1 --text "09:02 delivered invoice.html to j.doe" > /dev/null
C incident timeline add --at "2026-11-18 09:02" --text "Phishing mail delivered to j.doe" --evidence EV-1 --mark occurred > /dev/null
C incident timeline add --at "2026-11-18 09:30" --text "User reported the mail" --source j.doe --mark detected > /dev/null
C incident timeline add --at "2026-11-18 09:10" --text "Credentials entered on the page" --suspected > /dev/null
C incident investigation open --title "Were the credentials used?" --hypothesis "Logins from a new address" > /dev/null
C incident investigation note INV-1 --text "Sign-in log requested from the identity team." > /dev/null
C incident indicator add --type url --value "https://login.example.net/x" --context "phishing page" > /dev/null
C incident note situation --text "One mailbox was phished at 09:02. Whether the password was used is under investigation." > /dev/null
C incident note next-steps --text "Read the sign-in log; reset the password once approved." > /dev/null
C incident action propose --kind containment --title "Reset the password of j.doe" --detail "Reset the password and revoke every session of j.doe in the IdP." \
  --risk "The user is logged out everywhere." > /dev/null
C incident report sitrep > /dev/null

printf '{"task":"%s","done":"%s","shop":"%s","audit":"%s","desk":"%s","case":"%s","caseRoot":"%s","lacarta":"%s","home":"%s","shopRoot":"%s","gitlab":"%s"}\n' \
  "$TASK" "$DONE" "$SHOP" "$AUDIT" "$DESK" "$CASE" "$RUN/case" "$L" "$LACARTA_HOME" "$RUN/shop" "$LACARTA_FAKE_GLAB" > "$RUN/ids.json"
unset LACARTA_NO_DAEMON
exec "$L" serve --port "${PORT:-4799}"
