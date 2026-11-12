#!/usr/bin/env python3
"""lacarta: one console for every agent session and project on this machine.

A single daemon (`serve`) shows all projects. Claude Code sessions register themselves
through hooks (`install`): a session in a folder with a .lacarta/ software project joins
it as the orchestrator; any other session becomes a single-agent task. The console shows
each one's chat, questions and activity, and can drive a session remotely.

Machine-wide state lives in ~/.lacarta/ ($LACARTA_HOME): the project registry, settings
(models per kind of project) and single-agent tasks. A software project's state lives in
<project>/.lacarta/ (state.json, specs/, reviews/), and its gates refuse to move the project
forward while the framework's rules are not met. Every write happens under a file lock.
Standard library only.

Run `lacarta.py <command> --help` for the options of each command.
"""
import argparse
import contextlib
import datetime
import hashlib
import http.server
import json
import os
import secrets
import shlex
import signal
import socket
import subprocess
import sys
import threading
import time
import traceback
import urllib.parse
from pathlib import Path

try:
    import fcntl
except ImportError:  # Windows: no advisory locks, single-writer use only
    fcntl = None

SKILL_DIR = Path(__file__).resolve().parent.parent
TEMPLATES = SKILL_DIR / "assets" / "templates"
UI_DIR = SKILL_DIR / "assets" / "ui"
STATE_DIR = ".lacarta"
HOME = Path(os.environ.get("LACARTA_HOME") or "~/.lacarta").expanduser()
DEFAULT_PORT = 4747

SPECS = {
    "functional": "Functional",
    "architecture": "Architecture & design",
    "stack": "Tech stack",
    "security": "Security & threat model",
}
SEVERITIES = ["critical", "high", "medium", "low", "info"]
BLOCKING = {"critical", "high"}
REVIEW_TYPES = ["functional", "security"]          # every phase and the final review
SPEC_REVIEW = "specification"                       # the fresh-agent review of the whole spec set
ALL_REVIEW_TYPES = REVIEW_TYPES + [SPEC_REVIEW]
# A finding in one of these states still needs work.
UNRESOLVED = {"open", "fixing", "redesign"}

# Model choices map to the Agent tool's `model` aliases, which resolve to these releases.
MODELS = {
    "opus": {"name": "Opus 5.5", "id": "claude-opus-5-5"},
    "sonnet": {"name": "Sonnet 5.5", "id": "claude-sonnet-5-5"},
    "haiku": {"name": "Haiku 5.5", "id": "claude-haiku-5-5"},
    "fable": {"name": "Fable 5.1", "id": "claude-fable-5-1"},
}
# role: label, framework stage, what it does, default model
ROLES = {
    "orchestrator": ("Orchestrator", "Whole project",
                     "Runs the framework, writes the specs, talks to you and coordinates the other agents. "
                     "This is the Claude Code session itself: switch it with /model.", "opus"),
    "spec_review": ("Specification review", "Specification",
                    "Fresh agent that checks the complete spec set for gaps and incoherences.", "sonnet"),
    "implementation": ("Implementation", "Implementation",
                       "Subagent that builds each phase's scope and its tests.", "opus"),
    "functional_review": ("Functional review", "Implementation",
                          "Reviews each phase against the functional spec and architecture.", "sonnet"),
    "security_review": ("Security review", "Implementation",
                        "Reviews each phase against the threat model and hunts for vulnerabilities.", "opus"),
    "final_functional_review": ("Final functional review", "Final review",
                                "Fresh agent that judges the whole repository against the specs.", "sonnet"),
    "final_security_review": ("Final security review", "Final review",
                              "Fresh agent that audits the whole repository.", "opus"),
}
USE_MODELS = "Use these models"   # answer of the setup question older projects still carry

# Kinds of project. Each has its own console view and its own roles; the models for every
# role are set once, machine-wide, on the console's Settings page.
SINGLE, SOFTWARE = "single", "software"
KINDS = {
    SINGLE: {"label": "Single agent", "about": "One agent session on one task: its chat, questions and activity.",
             "roles": {"agent": ("Agent", "Session", "The CLI session itself. Switch it in the terminal "
                                 "(`/model` in Claude Code); helpers it spawns use this model too.", "opus")}},
    SOFTWARE: {"label": "Software development",
               "about": "Specs, a phase plan, phases gated by functional and security reviews, a final review.",
               "roles": ROLES},
}
PLANNED_KINDS = ["Security audit", "DevOps (Ansible, Terraform)"]

# Agents get a human name so the user knows who is asking. The orchestrator is always A-1.
AGENT_NAMES = ["Ada", "Basil", "Cleo", "Dorian", "Esme", "Felix", "Gaia", "Hugo", "Iris", "Jonah",
               "Kira", "Leon", "Mira", "Nico", "Opal", "Pavel", "Quinn", "Rosa", "Silas", "Tess",
               "Ugo", "Vera", "Wren", "Xavi", "Yara", "Zeno"]
ORCHESTRATOR = "A-1"
AGENT_STATES = ["working", "idle", "done", "failed"]
# Who is running this command: --agent, else $LACARTA_AGENT, else the orchestrator.
# The console server sets it to None: everything it records comes from the user.
ACTOR = ORCHESTRATOR

APPROVE = "Approve"
REQUEST_CHANGES = "Request changes"
TRIAGE_FIX = "Fix it now"
TRIAGE_REDESIGN = "Change the spec or design"
TRIAGE_ACCEPT = "Accept the risk"
TRIAGE_STOP = "Stop the project"

# What each fixed choice does, shown under the option in the console.
APPROVAL_HELP = {
    APPROVE: "Accept it as it is. The framework moves on to the next step.",
    REQUEST_CHANGES: "Send it back. Say what should change in the comment; the agent revises it and asks again.",
}
TRIAGE_HELP = {
    TRIAGE_FIX: "It is a bug in the code. The agent fixes only this finding, adds a test that proves the fix, "
                "and a fresh review checks it. The specs stay as they are. Work resumes once the fix is verified.",
    TRIAGE_REDESIGN: "Patching the code is not enough: the weakness comes from what was specified or designed. "
                     "The agent first updates the specs (asking you any choice that needs), then fixes the code, "
                     "and a fresh review checks it.",
    TRIAGE_ACCEPT: "Leave it unfixed and carry on. Your reason is recorded in the decision log and in the accepted "
                   "risks of the security spec, where the final reviewers will see it. A written reason is required.",
    TRIAGE_STOP: "Freeze the project as it is now. Nothing advances any more. Pick this when the finding puts "
                 "the whole approach in question.",
}
# What a severity means, per review type, so a triage question can be judged on its own.
SEVERITY_MEANING = {
    "security": {
        "critical": "exploitable remotely with no or trivial preconditions; full compromise, code execution, "
                    "broad data exposure or leaked production secrets.",
        "high": "serious impact with a realistic precondition (an authenticated user, a specific configuration): "
                "authorisation bypass, injection, account takeover, sensitive data exposure.",
    },
    "functional": {
        "critical": "data loss or corruption, or the phase's core goal does not work at all.",
        "high": "a must requirement is unmet or wrong in a common case.",
    },
}


class GateError(Exception):
    """A framework rule refuses the requested transition."""


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


# ---------------------------------------------------------------- storage

def find_root(explicit=None, start=None):
    if explicit:
        return Path(explicit).resolve()
    here = Path(start or Path.cwd()).resolve()
    for d in [here, *here.parents]:
        if (d / STATE_DIR / "state.json").exists():
            return d
    return None


@contextlib.contextmanager
def file_lock(path, exclusive):
    with open(path, "a+") as lf:
        if fcntl:
            fcntl.flock(lf, fcntl.LOCK_EX if exclusive else fcntl.LOCK_SH)
        yield


def write_json(path, data):
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(data, indent=2) + "\n")
    os.replace(tmp, path)


class Store:
    """One project's state. `root` is the folder the project is about; `dir` holds its state
    (<root>/.lacarta for a software project, ~/.lacarta/tasks/<id> for a single-agent task)."""

    def __init__(self, root, dir=None):
        self.root = Path(root)
        self.dir = Path(dir) if dir else self.root / STATE_DIR
        self.path = self.dir / "state.json"

    def _lock(self, exclusive):
        return file_lock(self.dir / ".lock", exclusive)

    def read(self):
        with self._lock(False):
            return migrate(json.loads(self.path.read_text()))

    @contextlib.contextmanager
    def edit(self):
        """Yield the state; it is written back only if the block succeeds."""
        with self._lock(True):
            state = migrate(json.loads(self.path.read_text()))
            yield state
            refresh(state, self)
            write_json(self.path, state)


def migrate(state):
    """Bring a state file written by an older version up to date."""
    state.setdefault("template", SOFTWARE)
    # Models moved to the machine-wide settings; the per-project setup question is obsolete.
    state.pop("models", None)
    for q in state.get("questions", []):
        if q["status"] == "open" and (q.get("effect") or {}).get("type") == "models-confirm":
            q["status"] = "superseded"
    state.setdefault("inbox", [])
    state.setdefault("remote", None)       # None: follow the machine-wide default
    state.setdefault("listening", None)
    state.setdefault("attention", None)
    if "agents" not in state:
        state["agents"] = []
        new_agent(state, "orchestrator", "Runs the framework and talks to you.", log_it=False)
    return state


# --------------------------------------------------------------------- hub
# ~/.lacarta/hub.json lists every project the console shows and maps CLI sessions to them.

def hub_path():
    return HOME / "hub.json"


def hub_read():
    if not hub_path().exists():
        return {"projects": {}, "sessions": {}}
    with file_lock(HOME / ".lock", False):
        return json.loads(hub_path().read_text())


@contextlib.contextmanager
def hub_edit():
    HOME.mkdir(parents=True, exist_ok=True)
    with file_lock(HOME / ".lock", True):
        hub = json.loads(hub_path().read_text()) if hub_path().exists() else {"projects": {}, "sessions": {}}
        yield hub
        write_json(hub_path(), hub)


def slug(text):
    out = "".join(c if c.isalnum() else "-" for c in text.lower()).strip("-")
    return "-".join(x for x in out.split("-") if x)[:32] or "project"


def register_software(root, name):
    """Add a software project to the hub (idempotent); returns its id."""
    root = Path(root).resolve()
    with hub_edit() as hub:
        for pid, p in hub["projects"].items():
            if p.get("root") == str(root):
                p["name"] = name
                return pid
        pid = f"{slug(name)}-{hashlib.sha256(str(root).encode()).hexdigest()[:4]}"
        hub["projects"][pid] = {"id": pid, "template": SOFTWARE, "name": name, "root": str(root),
                                "created_at": now(), "archived": False}
        return pid


def project_store(pid):
    p = hub_read()["projects"].get(pid)
    if not p:
        raise GateError(f"No project {pid} in {hub_path()}.")
    if p["template"] == SINGLE:
        return Store(p.get("cwd") or HOME, HOME / "tasks" / pid)
    return Store(p["root"])


def settings_path():
    return HOME / "settings.json"


def default_settings():
    return {"models": {k: {r: v[3] for r, v in kind["roles"].items()} for k, kind in KINDS.items()},
            "remote_control": True}


def settings_read():
    s = default_settings()
    if settings_path().exists():
        saved = json.loads(settings_path().read_text())
        for k, roles in (saved.get("models") or {}).items():
            if k in s["models"]:
                s["models"][k].update({r: m for r, m in roles.items() if r in s["models"][k] and m in MODELS})
        s["remote_control"] = bool(saved.get("remote_control", s["remote_control"]))
    return s


def settings_update(models=None, remote_control=None):
    with hub_edit():                      # the hub lock also guards settings.json
        s = settings_read()
        for k, roles in (models or {}).items():
            if k not in KINDS:
                raise GateError(f"Unknown kind of project {k}. Kinds: {', '.join(KINDS)}.")
            for r, m in roles.items():
                if r not in KINDS[k]["roles"]:
                    raise GateError(f"Unknown role {r} for {k}. Roles: {', '.join(KINDS[k]['roles'])}.")
                if m not in MODELS:
                    raise GateError(f"Unknown model {m}. Models: {', '.join(MODELS)}.")
                s["models"][k][r] = m
        if remote_control is not None:
            s["remote_control"] = bool(remote_control)
        write_json(settings_path(), s)
    return s


def model_for(state, role):
    return settings_read()["models"][state["template"]].get(role) or KINDS[state["template"]]["roles"][role][3]


def remote_on(state):
    return settings_read()["remote_control"] if state.get("remote") is None else state["remote"]


# ------------------------------------------------------------------ agents

def new_agent(state, role, task, phase=None, log_it=True):
    used = {a["name"] for a in state["agents"]}
    n = len(state["agents"])
    name = next((x for x in AGENT_NAMES if x not in used), f"{AGENT_NAMES[n % len(AGENT_NAMES)]} {n // len(AGENT_NAMES) + 1}")
    a = {
        "id": f"A-{n + 1}", "name": name, "role": role, "model": model_for(state, role),
        "task": task, "phase": phase, "status": "working", "summary": "",
        "avatar": None,  # reserved for a custom profile picture; the console shows initials meanwhile
        "started_at": now(), "updated_at": now(), "finished_at": None,
    }
    state["agents"].append(a)
    if log_it:
        log(state, f"{name} started as {role_label(state, role).lower()} on {MODELS[a['model']]['name']}: {task}",
            "agent")
    return a


def role_label(state, role):
    return KINDS[state["template"]]["roles"].get(role, (role,))[0]


def get_agent(state, aid):
    a = next((a for a in state["agents"] if a["id"] == aid), None)
    if not a:
        raise GateError(f"No agent {aid}. Agents: {', '.join(x['id'] for x in state['agents'])}.")
    return a


def actor(state):
    """The id of the agent running this command (None in the console server)."""
    if ACTOR is None:
        return None
    a = get_agent(state, ACTOR)
    if a["status"] in ("done", "failed"):
        raise GateError(f"{a['name']} ({a['id']}) is {a['status']}; start a new agent with `agent start`.")
    return a["id"]


def agent_status(state, a):
    """What the console shows: an agent with an unanswered question is waiting for the user."""
    if a["status"] in ("done", "failed"):
        return a["status"]
    if any(q["status"] == "open" and q.get("asked_by", ORCHESTRATOR) == a["id"] for q in state["questions"]):
        return "waiting"
    if a["id"] == ORCHESTRATOR:
        if state["stage"] == "done":
            return "done"
        if blocked_reason(state):
            return "blocked"
        if state.get("attention"):
            return "terminal"       # the CLI is waiting for the user at the terminal (a permission prompt)
        if listening(state):
            return "listening"      # turn finished; the session waits for a console message
    return a["status"]


def pid_alive(pid):
    try:
        os.kill(int(pid), 0)
        return True
    except (OSError, ValueError, TypeError):
        return False


def listening(state):
    lst = state.get("listening")
    return bool(lst) and pid_alive(lst.get("pid"))


def next_id(state, prefix):
    state["counters"][prefix] = state["counters"].get(prefix, 0) + 1
    return f"{prefix}-{state['counters'][prefix]}"


def log(state, text, kind="info"):
    agent = ACTOR if any(a["id"] == ACTOR for a in state.get("agents", [])) else None
    state["activity"].append({"at": now(), "kind": kind, "text": text, "agent": agent})
    del state["activity"][:-300]
    if agent:
        get_agent(state, agent)["updated_at"] = now()


# ------------------------------------------------------------ derived state

def blocking_findings(state):
    return [f for f in state["findings"]
            if f["severity"] in BLOCKING and f["status"] in UNRESOLVED]


def blocked_reason(state):
    if state["stage"] == "stopped":
        return "The project was stopped by a decision."
    fs = blocking_findings(state)
    if fs:
        ids = ", ".join(f"{f['id']} ({f['severity']})" for f in fs)
        return f"Unresolved high or critical finding: {ids}."
    return None


def phase(state, pid):
    for p in state["phases"]:
        if p["id"] == pid:
            return p
    raise GateError(f"No phase {pid}. Phases: {[p['id'] for p in state['phases']] or 'none yet'}.")


def latest_review(state, pid, rtype):
    rs = [r for r in state["reviews"] if r["phase"] == pid and r["type"] == rtype]
    return rs[-1] if rs else None


def open_question(state, effect):
    for q in state["questions"]:
        if q["status"] == "open" and q.get("effect") == effect:
            return q
    return None


def parse_pid(value):
    return value if value in ("spec", "final") else int(value)


def where(pid):
    return {"spec": "the specification", "final": "the final review"}.get(pid, f"phase {pid}")


# The spec review and the approvals are tied to the exact text they judged: any edit to a
# spec file or to the plan after that makes them stale.

def spec_file_hash(store, kind):
    return hashlib.sha256((store.dir / "specs" / f"{kind}.md").read_bytes()).hexdigest()


def plan_hash(state):
    plan = [[p["title"], p["goal"], p["scope"], p["covers"]] for p in state["phases"]]
    return hashlib.sha256(json.dumps(plan).encode()).hexdigest()


def specs_hash(state, store):
    parts = [spec_file_hash(store, k) for k in SPECS] + [plan_hash(state)]
    return hashlib.sha256("".join(parts).encode()).hexdigest()


def todo_lines(store, kind):
    text = (store.dir / "specs" / f"{kind}.md").read_text()
    return [i + 1 for i, line in enumerate(text.splitlines()) if "TODO" in line]


def spec_review_status(state, store):
    """'none', 'failed', 'stale' (specs changed since it passed) or 'passed'."""
    r = latest_review(state, "spec", SPEC_REVIEW)
    if not r:
        return "none"
    if r["verdict"] != "pass" or any(f["phase"] == "spec" and f["status"] in UNRESOLVED
                                     for f in state["findings"]):
        return "failed"
    return "passed" if r.get("specs_hash") == specs_hash(state, store) else "stale"


def supersede(state, effect, why):
    q = open_question(state, effect)
    if q:
        q["status"] = "superseded"
        log(state, f"{q['id']} withdrawn: {why}", "question")


def refresh(state, store):
    """Invalidate approvals of edited specs, then advance the stage when its exit conditions are met."""
    if state["stage"] != "specification":
        return
    for k, s in state["specs"].items():
        if s["status"] in ("in-review", "approved") and s.get("submitted_hash") != spec_file_hash(store, k):
            s["status"] = "changed"
            supersede(state, {"type": "spec-approval", "spec": k}, f"{k}.md changed after submission.")
            log(state, f"{SPECS[k]} spec edited after submission: needs a new spec review and approval.", "spec")
    plan = state["plan"]
    if plan["status"] in ("in-review", "approved") and plan.get("submitted_hash") != plan_hash(state):
        plan["status"] = "changed"
        supersede(state, {"type": "plan-approval"}, "the plan changed after submission.")
        log(state, "Phase plan edited after submission: needs a new spec review and approval.", "plan")
    if all(s["status"] == "approved" for s in state["specs"].values()) \
            and plan["status"] == "approved" and spec_review_status(state, store) == "passed":
        state["stage"] = "implementation"
        log(state, "Specification reviewed and approved. Implementation can start.", "stage")


def next_action(state, store):
    """One sentence telling an agent what the framework expects next."""
    stage = state["stage"]
    if stage == "stopped":
        return "The project is stopped. Nothing advances until the user restarts it."
    if pending_messages(state):
        return "The user sent messages from the console: `inbox` shows them. Treat them as the user's words."
    talk = [q for q in state["questions"] if discussing(q)]
    if talk:
        return (f"The user wants to talk about {talk[0]['id']} before answering: read the thread and "
                f"`reply {talk[0]['id']} --message ...`, then `wait` again. The question stays open.")
    if state["template"] == SINGLE:
        waiting = [q["id"] for q in state["questions"] if q["status"] == "open"]
        if stage == "ended":
            return "The session ended."
        return (f"Wait for the user to answer {', '.join(waiting)} in the console." if waiting
                else "Work on the task with the user.")
    triage = [q for q in state["questions"]
              if q["status"] == "open" and (q.get("effect") or {}).get("type") == "finding-triage"]
    if triage:
        return f"Blocked: wait for the user's decision on {triage[0]['id']} (`wait {triage[0]['id']}`). Do not write code."
    if blocking_findings(state):
        f = blocking_findings(state)[0]
        return (f"Blocked on {f['id']} ({f['status']}): fix it, re-run the {f['review']} review, "
                f"then `finding resolve {f['id']} --status fixed`. Nothing else advances.")
    waiting = [q for q in state["questions"] if q["status"] == "open"]
    if stage == "specification":
        incomplete = [k for k in SPECS if todo_lines(store, k)]
        if incomplete:
            return f"Finish writing the specs: {', '.join(f'.lacarta/specs/{k}.md' for k in incomplete)} still have TODOs."
        if not state["phases"]:
            return "Define the phase plan with `phase add`."
        talks = [q for q in waiting if q["kind"] != "validation"]
        if talks:
            return f"Wait for the user to answer {', '.join(q['id'] for q in talks)}, then fold the answers into the specs."
        review = spec_review_status(state, store)
        if review != "passed":
            hint = {"none": "Everything is written", "failed": "The last spec review failed: fix its findings",
                    "stale": "The specs changed since the last spec review"}[review]
            return (f"{hint}. Spawn a fresh spec reviewer with no memory (references/spec-review.md) and "
                    "record it with `review add --phase spec --type specification`.")
        unsubmitted = [k for k, s in state["specs"].items()
                       if s["status"] in ("draft", "changes-requested", "changed")]
        if unsubmitted:
            return f"Spec review passed: submit for approval with `spec submit` ({', '.join(unsubmitted)})."
        if state["plan"]["status"] in ("draft", "changes-requested", "changed"):
            return "Spec review passed: `plan submit`."
        if waiting:
            return f"Wait for the user to answer {', '.join(q['id'] for q in waiting)} in the console."
        return "Specifications are in review."
    if stage == "implementation":
        cur = [p for p in state["phases"] if p["status"] not in ("planned", "done")]
        if cur:
            p = cur[0]
            if p["status"] == "awaiting-approval":
                return f"Wait for the user to approve phase {p['id']} in the console."
            missing = [t for t in REVIEW_TYPES
                       if not (latest_review(state, p["id"], t) or {}).get("verdict") == "pass"]
            if missing:
                return (f"Phase {p['id']} in progress: finish its scope, then run the "
                        f"{' and '.join(missing)} review(s) and record them with `review add`.")
            return f"Phase {p['id']} reviews passed: `phase complete {p['id']}`."
        nxt = [p for p in state["phases"] if p["status"] == "planned"]
        if nxt:
            return f"`phase start {nxt[0]['id']}`."
        return "All phases done: `final start`, then run the final review with fresh agents."
    if stage == "final-review":
        if open_question(state, {"type": "final-approval"}):
            return "Wait for the user's final approval in the console."
        missing = [t for t in REVIEW_TYPES
                   if not (latest_review(state, "final", t) or {}).get("verdict") == "pass"]
        if missing:
            return (f"Run the final {' and '.join(missing)} review(s) with fresh agents "
                    "(references/final-review.md), then fix and re-review until they pass.")
        return "Final reviews passed: `final complete`."
    return "The project is done."


def derived(state, store):
    common = {
        "next": next_action(state, store),
        "open_questions": sum(q["status"] == "open" for q in state["questions"]),
        "agent_status": {a["id"]: agent_status(state, a) for a in state["agents"]},
        "listening": listening(state),
        "remote": remote_on(state),
        "pending_messages": len(pending_messages(state)),
    }
    if state["template"] == SINGLE:
        return {**common, "blocked": None}
    sev = {s: 0 for s in SEVERITIES}
    for f in state["findings"]:
        if f["status"] in UNRESOLVED:
            sev[f["severity"]] += 1
    return {
        **common,
        "blocked": blocked_reason(state),
        "spec_review": spec_review_status(state, store),
        "open_findings": sev,
        "specs_approved": sum(s["status"] == "approved" for s in state["specs"].values()),
        "phases_done": sum(p["status"] == "done" for p in state["phases"]),
    }


# --------------------------------------------------------------- questions

def add_question(state, kind, title, body="", options=None, effect=None, context=None, allow_free=True,
                 option_help=None):
    if effect:
        existing = open_question(state, effect)
        if existing:
            return existing
    q = {
        "id": next_id(state, "Q"), "kind": kind, "title": title, "body": body,
        "options": options or [], "option_help": option_help or {}, "allow_free": allow_free,
        "context": context or {}, "effect": effect, "status": "open", "asked_at": now(), "answer": None,
        "asked_by": actor(state) or ORCHESTRATOR, "thread": [],
    }
    state["questions"].append(q)
    log(state, f"{q['id']} asked: {title}", "question")
    return q


def get_question(state, qid):
    q = next((q for q in state["questions"] if q["id"] == qid), None)
    if not q:
        raise GateError(f"No question {qid}.")
    return q


def parse_options(values):
    """`--option "Label|What it means"` values -> (labels, {label: help})."""
    labels, helps = [], {}
    for v in values or []:
        label, _, text = v.partition("|")
        label = label.strip()
        if not label:
            raise GateError(f"Empty option label in {v!r}.")
        labels.append(label)
        if text.strip():
            helps[label] = text.strip()
    return labels, helps


# "Talk about this": the user can discuss an open question with the agent before answering it.
# The question stays open; the agent owes a reply while the user spoke last.

def discussing(q):
    thread = q.get("thread") or []
    return q["status"] == "open" and bool(thread) and thread[-1]["by"] == "user"


def discuss_question(state, qid, text):
    q = get_question(state, qid)
    if q["status"] != "open":
        raise GateError(f"{qid} is already answered.")
    text = (text or "").strip()
    if not text:
        raise GateError("Write a message.")
    q.setdefault("thread", []).append({"by": "user", "text": text, "at": now()})
    log(state, f"{qid}: you asked the agent about it.", "question")
    return q


def answer_question(state, qid, choice, comment="", by="user"):
    q = get_question(state, qid)
    if q["status"] != "open":
        raise GateError(f"{qid} is already answered.")
    choice = (choice or "").strip()
    comment = (comment or "").strip()
    if q["options"] and choice not in q["options"]:
        if not (q["allow_free"] and not choice and comment):
            raise GateError(f"Choose one of {q['options']}" + (" or write an answer." if q["allow_free"] else "."))
    if not q["options"] and not comment and not choice:
        raise GateError("Write an answer.")
    if (q.get("effect") or {}).get("type") == "finding-triage" and choice == TRIAGE_ACCEPT and not comment:
        raise GateError("Accepting a high or critical risk needs a written rationale.")
    q["status"] = "answered"
    q["answer"] = {"choice": choice, "comment": comment, "by": by, "at": now()}
    ctx = q["context"]
    decision = choice or comment
    state["decisions"].append({
        "id": next_id(state, "D"), "title": q["title"], "decision": decision,
        "rationale": comment if choice else "", "source": qid, "by": by,
        "spec": ctx.get("spec"), "phase": ctx.get("phase"), "at": now(),
    })
    log(state, f"{qid} answered: {decision}", "decision")
    apply_effect(state, q)
    return q


def apply_effect(state, q):
    eff = q.get("effect")
    if not eff:
        return
    choice, comment = q["answer"]["choice"], q["answer"]["comment"]
    t = eff["type"]
    if t == "spec-approval":
        s = state["specs"][eff["spec"]]
        if choice == APPROVE:
            s.update(status="approved", approved_at=now(), approved_revision=s["revision"])
        else:
            s["status"] = "changes-requested"
        log(state, f"{SPECS[eff['spec']]} spec {s['status']}.", "spec")
    elif t == "plan-approval":
        state["plan"]["status"] = "approved" if choice == APPROVE else "changes-requested"
        state["plan"]["approved_at"] = now() if choice == APPROVE else None
        log(state, f"Phase plan {state['plan']['status']}.", "plan")
    elif t == "phase-approval":
        p = phase(state, eff["phase"])
        if choice == APPROVE:
            p.update(status="done", completed_at=now())
            log(state, f"Phase {p['id']} done.", "phase")
        else:
            p["status"] = "in-progress"
            log(state, f"Phase {p['id']} sent back: changes requested.", "phase")
    elif t == "final-approval":
        if choice == APPROVE:
            state["stage"] = "done"
            log(state, "Final review approved. Project done.", "stage")
        else:
            log(state, "Final approval refused: changes requested.", "stage")
    elif t == "finding-triage":
        f = next(f for f in state["findings"] if f["id"] == eff["finding"])
        if choice == TRIAGE_ACCEPT:
            f.update(status="accepted", resolution=comment, resolved_at=now())
        elif choice == TRIAGE_REDESIGN:
            f["status"] = "redesign"
        elif choice == TRIAGE_STOP:
            state["stage"] = "stopped"
        else:
            f["status"] = "fixing"
        log(state, f"{f['id']} triaged: {choice or comment}.", "finding")


# ---------------------------------------------------------------- sessions
# A CLI session (Claude Code for now) registers through hooks and is attached to agent A-1 of
# its project. Messages typed in the console wait in the project's inbox until a hook hands
# them to the agent: after its next tool call, or when it ends its turn.

CONSOLE_TAG = "[lacarta console]"
LISTEN_MAX = 23 * 3600          # a listening Stop hook gives up before its 24 h hook timeout
CLI_CLAUDE = "claude-code"


def pending_messages(state):
    return [m for m in state.get("inbox", []) if not m.get("delivered_at")]


def add_message(state, text):
    text = (text or "").strip()
    if not text:
        raise GateError("Write a message.")
    a = get_agent(state, ORCHESTRATOR)
    if not a.get("session") or a["session"].get("ended_at"):
        raise GateError("No agent session is connected to this project right now, so nothing would read it.")
    m = {"id": next_id(state, "M"), "text": text, "at": now(), "delivered_at": None}
    state["inbox"].append(m)
    del state["inbox"][:-200]
    log(state, "You sent a message from the console.", "chat")
    return m


def take_messages(state):
    """Mark the pending messages delivered; returns them as one prompt for the agent ("" if none)."""
    ms = pending_messages(state)
    for m in ms:
        m["delivered_at"] = now()
    if not ms:
        return ""
    return (f"{CONSOLE_TAG} The user wrote from the lacarta console. Treat it exactly as if they had typed it "
            "here:\n\n" + "\n\n".join(m["text"] for m in ms))


def withdraw_open_questions(state, why):
    """Nobody is left to act on these answers."""
    for q in state["questions"]:
        if q["status"] == "open":
            q["status"] = "superseded"
            log(state, f"{q['id']} withdrawn: {why}", "question")


def create_single(cwd):
    tid = "t-" + secrets.token_hex(3)
    d = HOME / "tasks" / tid
    d.mkdir(parents=True)
    name = Path(cwd).name or "Task"
    state = {
        "version": 1, "template": SINGLE,
        "project": {"name": name, "summary": "", "cwd": str(cwd), "created_at": now(), "named": False},
        "stage": "active", "agents": [], "questions": [], "decisions": [], "findings": [], "phases": [],
        "reviews": [], "specs": {}, "plan": {}, "activity": [], "counters": {},
    }
    migrate(state)
    new_agent(state, "agent", "Works on the task with you.", log_it=False)
    log(state, f"Task started in {cwd}.", "stage")
    write_json(d / "state.json", state)
    with hub_edit() as hub:
        hub["projects"][tid] = {"id": tid, "template": SINGLE, "name": name, "cwd": str(cwd),
                                "created_at": now(), "archived": False}
    return tid, Store(cwd, d)


def attach_session(store, sid, cli, transcript, cwd, source=""):
    with store.edit() as state:
        a = get_agent(state, ORCHESTRATOR)
        a["session"] = {"cli": cli, "id": sid, "transcript": transcript, "cwd": str(cwd),
                        "started_at": now(), "ended_at": None}
        a.update(status="working", updated_at=now())
        state["attention"] = None
        if state["template"] == SINGLE and state["stage"] == "ended":
            state["stage"] = "active"
            a.update(finished_at=None, summary="")
        log(state, f"{a['name']} connected ({cli} session{', ' + source if source else ''}).", "agent")


def register_session(payload, cli=CLI_CLAUDE):
    """Find or create the project of a CLI session; returns (project id, store)."""
    sid = str(payload.get("session_id") or "")
    cwd = payload.get("cwd") or os.getcwd()
    hub = hub_read()
    pid = hub["sessions"].get(sid)
    if pid in hub["projects"]:
        return pid, project_store(pid)
    root = find_root(None, cwd)
    if root:
        name = json.loads((root / STATE_DIR / "state.json").read_text())["project"]["name"]
        pid, store = register_software(root, name), Store(root)
    else:
        pid, store = create_single(cwd)
    with hub_edit() as hub:
        hub["sessions"][sid] = pid
        for old in list(hub["sessions"])[:-500]:
            del hub["sessions"][old]
    attach_session(store, sid, cli, payload.get("transcript_path"), cwd, payload.get("source", ""))
    return pid, store


def session_store(payload):
    pid = hub_read()["sessions"].get(str(payload.get("session_id") or ""))
    if pid:
        try:
            return project_store(pid)
        except GateError:
            pass
    return register_session(payload)[1]   # hooks installed mid-session: register on first contact


def move_session(sid, to_pid, to_store):
    """`init` inside a registered single-agent session: the session now belongs to the new project."""
    hub = hub_read()
    old = hub["sessions"].get(sid)
    if not old or old == to_pid or hub["projects"].get(old, {}).get("template") != SINGLE:
        return
    old_store = project_store(old)
    with old_store.edit() as state:
        a = get_agent(state, ORCHESTRATOR)
        sess = dict(a.get("session") or {})
        a.update(status="done", finished_at=now(), summary=f"Continued as the software project {to_pid}.")
        state["stage"] = "ended"
        state["moved_to"] = to_pid
        withdraw_open_questions(state, "the session moved to a software project.")
        log(state, f"The session became the software project {to_pid}.", "stage")
    with hub_edit() as hub:
        hub["sessions"][sid] = to_pid
        hub["projects"][old]["archived"] = True
    attach_session(to_store, sid, sess.get("cli", CLI_CLAUDE), sess.get("transcript"), sess.get("cwd") or to_store.root)


# Transcripts: Claude Code appends one JSON object per line. The console shows the user's and
# the agent's text and one line per tool call; the CLI's own plumbing is left out.

_chat_cache, _chat_lock = {}, threading.Lock()
CLI_PLUMBING = ("<command-", "<local-command", "<system-reminder", "<bash-", "<user-memory", "Caveat:")


def transcript_allowed(path):
    """Only session transcripts are served, whatever a state file claims."""
    base = Path(os.environ.get("CLAUDE_CONFIG_DIR") or "~/.claude").expanduser().resolve()
    try:
        p = Path(path).resolve()
    except (TypeError, OSError):
        return False
    return p.suffix == ".jsonl" and p.is_relative_to(base) and p.is_file()


def chat_items(line):
    try:
        e = json.loads(line)
    except ValueError:
        return []
    if e.get("isSidechain") or e.get("isMeta") or e.get("type") not in ("user", "assistant"):
        return []
    content = (e.get("message") or {}).get("content")
    blocks = [{"type": "text", "text": content}] if isinstance(content, str) else (content or [])
    out, at, who = [], e.get("timestamp"), e["type"]
    for b in blocks:
        if not isinstance(b, dict):
            continue
        if b.get("type") == "text":
            text = (b.get("text") or "").strip()
            if not text or (who == "user" and (text.startswith(CLI_PLUMBING) or CONSOLE_TAG in text)):
                continue    # console messages are shown from the inbox, with their delivery state
            out.append({"role": "user" if who == "user" else "agent", "text": text, "at": at})
        elif b.get("type") == "tool_use" and who == "assistant":
            inp = b.get("input") or {}
            hint = next((inp[k] for k in ("description", "command", "file_path", "pattern", "url", "prompt")
                         if isinstance(inp.get(k), str) and inp.get(k)), "")
            out.append({"role": "tool", "name": b.get("name", ""), "text": hint[:240], "at": at})
    return out


def read_chat(path, limit=400):
    if not path or not transcript_allowed(path):
        return []
    with _chat_lock:
        st = os.stat(path)
        c = _chat_cache.get(path)
        if not c or c["ino"] != st.st_ino or c["offset"] > st.st_size:
            c = {"ino": st.st_ino, "offset": 0, "items": []}
        with open(path, "rb") as f:
            f.seek(c["offset"])
            data = f.read()
        end = data.rfind(b"\n") + 1      # a line still being written waits for the next read
        for line in data[:end].splitlines():
            c["items"].extend(chat_items(line))
        del c["items"][:-limit]
        c["offset"] += end
        _chat_cache[path] = c
        return list(c["items"])


# Hooks: `lacarta.py hook <event>` with the CLI's JSON on stdin. A hook never fails the session:
# errors go to ~/.lacarta/hooks.log and the hook exits 0.

def hook_print(event, context=None, block=None):
    if block:
        print(json.dumps({"decision": "block", "reason": block}))
    elif context:
        print(json.dumps({"hookSpecificOutput": {"hookEventName": event, "additionalContext": context}}))


def session_context(pid, store):
    state = store.read()
    info = daemon_info()
    url = info["open_url"] if info else "not running (start it with `serve`)"
    L = f"python3 {Path(__file__).resolve()}"
    kind = KINDS[state["template"]]["label"].lower()
    lines = [f"lacarta: this session is registered in the lacarta console ({url}) as the {kind} project "
             f"\"{state['project']['name']}\" ({pid}). The user can follow it there and write to you from it: "
             f"those messages start with \"{CONSOLE_TAG}\" and are the user's own words. If `$LACARTA_PROJECT` "
             f"is empty in your shell, pass `--project {pid}` to lacarta commands."]
    if state["template"] == SOFTWARE:
        lines.append(f"This folder is a lacarta software project: load the `lacarta` skill now and follow it. "
                     f"Run `{L} status` first.")
    else:
        lines.append("If the user asks to build a new project, app or service, or wants specs or a structured, "
                     "reviewed process, load the `lacarta` skill. When you need a decision while the user may be "
                     f"away from the terminal, ask in the console: `{L} ask --title \"...\" --option "
                     f"\"Label|what it means\"`, then run `{L} wait` as a background command.")
    qs = [q["id"] for q in state["questions"] if q["status"] == "open"]
    if qs:
        lines.append(f"Questions still open in the console: {', '.join(qs)}.")
    return "\n".join(lines)


def hook_session_start(payload):
    ensure_daemon()
    pid, store = register_session(payload)
    env = os.environ.get("CLAUDE_ENV_FILE")
    if env:     # later Bash commands in this session resolve the project without a .lacarta/ folder
        with open(env, "a") as f:
            f.write(f"export LACARTA_PROJECT={shlex.quote(pid)}\n"
                    f"export LACARTA_SESSION={shlex.quote(str(payload.get('session_id') or ''))}\n")
    hook_print("SessionStart", session_context(pid, store))


def hook_prompt(payload):
    with session_store(payload).edit() as state:
        state["attention"] = None
        get_agent(state, ORCHESTRATOR).update(status="working", updated_at=now())
        prompt = str(payload.get("prompt") or "").strip()
        if state["template"] == SINGLE and not state["project"].get("named") and prompt \
                and not prompt.startswith("/"):
            first = prompt.splitlines()[0]
            state["project"].update(name=first[:70] + ("…" if len(first) > 70 else ""), named=True)


def hook_post_tool(payload):
    store = session_store(payload)
    state = store.read()
    if not (pending_messages(state) or state.get("attention")):
        return
    with store.edit() as state:
        state["attention"] = None
        msg = take_messages(state)
    hook_print("PostToolUse", msg)


def hook_notification(payload):
    with session_store(payload).edit() as state:
        text = str(payload.get("message") or "The agent needs you in the terminal.")
        state["attention"] = {"text": text, "at": now()}
        log(state, f"Needs you in the terminal: {text}", "agent")


def hook_stop(payload):
    """End of a turn: deliver console messages, or keep listening for one (remote control)."""
    store = session_store(payload)
    with store.edit() as state:
        state["attention"] = None
        a = get_agent(state, ORCHESTRATOR)
        msg = take_messages(state)
        listen = not msg and remote_on(state) and state["stage"] not in ("ended", "done")
        a.update(status="working" if msg else "idle", updated_at=now())
        if listen:
            state["listening"] = {"since": now(), "pid": os.getpid()}
        answered = {q["id"] for q in state["questions"] if q["status"] != "open"}
        threads = {q["id"]: len(q.get("thread") or []) for q in state["questions"]}
    if msg or not listen:
        return hook_print("Stop", block=msg or None)
    reason, deadline = None, time.time() + LISTEN_MAX
    try:
        while time.time() < deadline and not reason:
            time.sleep(1)
            state = store.read()
            if not remote_on(state):
                break
            if pending_messages(state):
                with store.edit() as state:
                    reason = take_messages(state)
                break
            news = []
            for q in state["questions"]:
                if q["id"] not in answered and q["status"] == "answered":
                    news.append(f"{q['id']} ({q['title']}) was answered: " + json.dumps(q["answer"]))
                elif discussing(q) and len(q["thread"]) > threads.get(q["id"], 0):
                    news.append(f"On {q['id']} ({q['title']}) the user chose \"Talk about this\" and wrote: "
                                f"{q['thread'][-1]['text']}\nReply with `reply {q['id']} --message ...`; "
                                "it is not answered.")
            if news:
                reason = (f"{CONSOLE_TAG} Activity in the lacarta console (a background `wait` may report the "
                          "same thing; it is the same event):\n\n" + "\n\n".join(news))
    finally:
        with store.edit() as state:
            state["listening"] = None
            if reason:
                get_agent(state, ORCHESTRATOR).update(status="working", updated_at=now())
    hook_print("Stop", block=reason)


def hook_session_end(payload):
    with session_store(payload).edit() as state:
        a = get_agent(state, ORCHESTRATOR)
        if a.get("session"):
            a["session"]["ended_at"] = now()
        state.update(listening=None, attention=None)
        if state["template"] == SINGLE:
            state["stage"] = "ended"
            a.update(status="done", finished_at=now(), updated_at=now())
            withdraw_open_questions(state, "the session ended.")
        else:
            a.update(status="idle", updated_at=now())
        log(state, f"{a['name']}'s session ended ({payload.get('reason') or 'exit'}).", "agent")


HOOKS = {"session-start": hook_session_start, "prompt": hook_prompt, "post-tool": hook_post_tool,
         "notification": hook_notification, "stop": hook_stop, "session-end": hook_session_end}
# Claude Code event: (our hook name, timeout in seconds, matcher)
CLAUDE_EVENTS = {
    "SessionStart": ("session-start", 30, None), "UserPromptSubmit": ("prompt", 15, None),
    "PostToolUse": ("post-tool", 15, "*"), "Notification": ("notification", 15, None),
    "Stop": ("stop", 86400, None), "SessionEnd": ("session-end", 15, None),
}
HOOK_MARK = "lacarta.py hook"


def cmd_hook(args):
    global ACTOR
    ACTOR = ORCHESTRATOR
    try:
        payload = json.loads(sys.stdin.read() or "{}")
    except ValueError:
        payload = {}
    # Esc in the CLI terminates a listening Stop hook: unwind so it stops showing as listening.
    signal.signal(signal.SIGTERM, lambda *a: sys.exit(0))
    try:
        HOOKS[args.event](payload)
    except Exception:
        with contextlib.suppress(OSError):
            HOME.mkdir(parents=True, exist_ok=True)
            with open(HOME / "hooks.log", "a") as f:
                f.write(f"{now()} {args.event}\n{traceback.format_exc()}\n")
    sys.exit(0)


def cmd_install(args):
    path = Path(args.settings or "~/.claude/settings.json").expanduser()
    cfg = json.loads(path.read_text()) if path.exists() and path.read_text().strip() else {}
    hooks = cfg.setdefault("hooks", {})
    base = f"python3 {shlex.quote(str(Path(__file__).resolve()))} hook"
    for event, (name, timeout, matcher) in CLAUDE_EVENTS.items():
        groups = [g for g in hooks.get(event, [])
                  if not any(HOOK_MARK in h.get("command", "") for h in g.get("hooks", []))]
        if not args.uninstall:
            g = {"hooks": [{"type": "command", "command": f"{base} {name}", "timeout": timeout}]}
            if matcher:
                g["matcher"] = matcher
            groups.append(g)
        if groups:
            hooks[event] = groups
        else:
            hooks.pop(event, None)
    if not hooks:
        cfg.pop("hooks")
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        path.with_name(path.name + ".bak").write_text(path.read_text())
    path.write_text(json.dumps(cfg, indent=2) + "\n")
    if args.uninstall:
        print(f"Removed the lacarta hooks from {path} (backup: {path.name}.bak).")
        return
    print(f"Installed the lacarta hooks in {path} (backup: {path.name}.bak). New Claude Code sessions "
          "register in the console; restart running sessions to include them.")
    skill = path.parent / "skills" / "lacarta"
    if not skill.exists():
        print(f"The skill itself is not in {skill.parent}: `ln -s {SKILL_DIR} {skill}` so sessions can load it.")


# The daemon: one per machine, started by the first session that needs it.

def daemon_info():
    f = HOME / "daemon.json"
    try:
        info = json.loads(f.read_text())
    except (OSError, ValueError):
        return None
    return info if pid_alive(info.get("pid")) else None


def ensure_daemon():
    info = daemon_info()
    if info or os.environ.get("LACARTA_NO_DAEMON"):    # tests run without a console
        return info
    HOME.mkdir(parents=True, exist_ok=True)
    with open(HOME / "daemon.log", "a") as log_file:
        subprocess.Popen([sys.executable, str(Path(__file__).resolve()), "serve"], cwd=str(HOME),
                         stdin=subprocess.DEVNULL, stdout=log_file, stderr=log_file, start_new_session=True)
    for _ in range(50):
        time.sleep(0.1)
        info = daemon_info()
        if info:
            return info
    return None


# ---------------------------------------------------------------- commands

def cmd_init(args):
    root = Path(args.root or ".").resolve()
    d = root / STATE_DIR
    if (d / "state.json").exists():
        raise GateError(f"{d} already exists. Use `status` to resume.")
    args.name = args.name or root.name
    (d / "specs").mkdir(parents=True, exist_ok=True)
    (d / "reviews").mkdir(exist_ok=True)
    for k in SPECS:
        target = d / "specs" / f"{k}.md"
        if not target.exists():
            target.write_text((TEMPLATES / f"{k}.md").read_text().replace("{{PROJECT}}", args.name))
    (d / ".gitignore").write_text(".lock\nstate.tmp\n")
    state = {
        "version": 1,
        "project": {"name": args.name, "summary": args.summary or "", "created_at": now()},
        "stage": "specification",
        "specs": {k: {"title": v, "status": "draft", "revision": 0, "approved_at": None,
                      "approved_revision": None} for k, v in SPECS.items()},
        "plan": {"status": "draft", "approved_at": None},
        "phases": [], "reviews": [], "questions": [], "decisions": [], "findings": [],
        "activity": [], "counters": {}, "template": SOFTWARE,
    }
    migrate(state)  # registers the orchestrator
    log(state, f"Project {args.name} initialised.", "stage")
    (d / "state.json").write_text(json.dumps(state, indent=2) + "\n")
    pid = register_software(root, args.name)
    sid = os.environ.get("LACARTA_SESSION")
    if sid:
        move_session(sid, pid, Store(root))
    info = ensure_daemon()
    print(f"Initialised {d} as project {pid}\nSpec templates: {d / 'specs'}\n"
          f"Console: {info['open_url'] if info else 'not running; start it with `serve`'}\n"
          "Next: tell the user the console URL, then ask what the project is (unless they already said).")


def cmd_status(args, store):
    state = store.read()
    if args.json:
        print(json.dumps({**state, "derived": derived(state, store)}, indent=2))
        return
    d = derived(state, store)
    if state["template"] == SINGLE:
        print(f"{state['project']['name']}: single-agent task in {state['project'].get('cwd')}, {state['stage']}")
        print(f"Remote control: {'on' if d['remote'] else 'off'}; console messages waiting: {d['pending_messages']}")
        for q in [q for q in state["questions"] if q["status"] == "open"]:
            print(f"  {q['id']} [{q['kind']}] {q['title']}")
        print(f"\nNext: {d['next']}")
        return
    print(f"{state['project']['name']}: stage {state['stage']}")
    if d["blocked"]:
        print(f"BLOCKED: {d['blocked']}")
    print("\nSpecs:")
    for k, s in state["specs"].items():
        todo = f"  ({len(todo_lines(store, k))} TODO lines)" if todo_lines(store, k) else ""
        print(f"  {k:<13} {s['status']:<18} rev {s['revision']}{todo}")
    print(f"\nSpec review: {d['spec_review']}")
    print(f"\nPhase plan: {state['plan']['status']}")
    for p in state["phases"]:
        revs = " ".join(f"{t}:{(latest_review(state, p['id'], t) or {}).get('verdict', '-')}" for t in REVIEW_TYPES)
        print(f"  {p['id']}. {p['title']:<40} {p['status']:<18} {revs}")
    qs = [q for q in state["questions"] if q["status"] == "open"]
    if qs:
        print("\nOpen questions:")
        for q in qs:
            print(f"  {q['id']} [{q['kind']}] {q['title']}")
    fs = [f for f in state["findings"] if f["status"] in UNRESOLVED]
    if fs:
        print("\nUnresolved findings:")
        for f in fs:
            print(f"  {f['id']} {f['severity']:<8} {f['status']:<9} phase {f['phase']}: {f['title']}")
    print(f"\nNext: {d['next']}")


def cmd_spec_submit(args, store):
    with store.edit() as state:
        if state["stage"] != "specification":
            raise GateError("Specs are submitted during the specification stage. "
                            "Record later changes with `decide --spec` and edit the file.")
        s = state["specs"][args.kind]
        if s["status"] in ("approved", "in-review"):
            raise GateError(f"The {args.kind} spec is already {s['status']}.")
        require_spec_review(state, store)
        s["revision"] += 1
        s["status"] = "in-review"
        s["submitted_hash"] = spec_file_hash(store, args.kind)
        q = add_question(
            state, "validation", f"Approve the {SPECS[args.kind]} spec (revision {s['revision']})?",
            args.note or f"Read it under Specifications → {SPECS[args.kind]}.",
            [APPROVE, REQUEST_CHANGES], {"type": "spec-approval", "spec": args.kind},
            {"stage": "specification", "spec": args.kind}, allow_free=False, option_help=APPROVAL_HELP)
    print(f"{q['id']} asks the user to approve the {args.kind} spec. `wait {q['id']}`.")


def require_spec_review(state, store):
    """Approvals are only asked for a complete spec set that a fresh reviewer passed as it is now."""
    for k in SPECS:
        if todo_lines(store, k):
            raise GateError(f"{k}.md still has TODO markers on lines {todo_lines(store, k)[:12]}. "
                            "The whole specification must be written before review and approval.")
    status = spec_review_status(state, store)
    if status != "passed":
        raise GateError({
            "none": "No specification review yet.",
            "failed": "The latest specification review failed or has unresolved findings.",
            "stale": "The specs or the plan changed after the last specification review passed.",
        }[status] + " Run a fresh spec reviewer (references/spec-review.md) first.")


def cmd_phase_add(args, store):
    with store.edit() as state:
        if state["plan"]["status"] in ("approved", "in-review"):
            raise GateError("The phase plan is approved. Changing it is a decision: `ask` the user first, "
                            "then `plan reopen`.")
        pid = len(state["phases"]) + 1
        state["phases"].append({
            "id": pid, "title": args.title, "goal": args.goal, "scope": args.scope or [],
            "covers": args.covers or [], "status": "planned", "started_at": None, "completed_at": None,
        })
        log(state, f"Phase {pid} planned: {args.title}", "plan")
    print(f"Phase {pid} added.")


def cmd_plan_submit(args, store):
    with store.edit() as state:
        if not state["phases"]:
            raise GateError("Add phases with `phase add` first.")
        if state["plan"]["status"] in ("approved", "in-review"):
            raise GateError(f"The plan is already {state['plan']['status']}.")
        if state["stage"] == "specification":
            require_spec_review(state, store)
        state["plan"]["status"] = "in-review"
        state["plan"]["submitted_hash"] = plan_hash(state)
        lines = [f"{p['id']}. **{p['title']}**: {p['goal']}" for p in state["phases"]]
        q = add_question(state, "validation", f"Approve the phase plan ({len(state['phases'])} phases)?",
                         "\n".join(lines), [APPROVE, REQUEST_CHANGES], {"type": "plan-approval"},
                         {"stage": "specification"}, allow_free=False, option_help=APPROVAL_HELP)
    print(f"{q['id']} asks the user to approve the plan. `wait {q['id']}`.")


def cmd_plan_reopen(args, store):
    with store.edit() as state:
        state["plan"]["status"] = "draft"
        log(state, f"Phase plan reopened: {args.reason}", "plan")
    print("Plan reopened. Edit phases, then `plan submit`.")


def cmd_phase_start(args, store):
    with store.edit() as state:
        reason = blocked_reason(state)
        if reason:
            raise GateError(f"Blocked. {reason}")
        if state["stage"] != "implementation":
            raise GateError(f"Phases start in the implementation stage (now: {state['stage']}).")
        if state["plan"]["status"] != "approved":
            raise GateError("The phase plan was reopened and is not approved again yet.")
        p = phase(state, args.id)
        if p["status"] != "planned":
            raise GateError(f"Phase {p['id']} is {p['status']}.")
        active = [x for x in state["phases"] if x["status"] in ("in-progress", "awaiting-approval")]
        if active:
            raise GateError(f"Phase {active[0]['id']} is still {active[0]['status']}. One phase at a time.")
        earlier = [x for x in state["phases"] if x["id"] < p["id"] and x["status"] != "done"]
        if earlier:
            raise GateError(f"Phase {earlier[0]['id']} is not done yet.")
        p.update(status="in-progress", started_at=now())
        log(state, f"Phase {p['id']} started: {p['title']}", "phase")
    print(f"Phase {args.id} started.")


def cmd_phase_complete(args, store):
    with store.edit() as state:
        reason = blocked_reason(state)
        if reason:
            raise GateError(f"Blocked. {reason}")
        p = phase(state, args.id)
        if p["status"] != "in-progress":
            raise GateError(f"Phase {p['id']} is {p['status']}, not in-progress.")
        for t in REVIEW_TYPES:
            r = latest_review(state, p["id"], t)
            if not r:
                raise GateError(f"No {t} review recorded for phase {p['id']}.")
            if r["verdict"] != "pass":
                raise GateError(f"The latest {t} review of phase {p['id']} ({r['id']}) failed. "
                                "Fix and review again.")
        left = [f["id"] for f in state["findings"] if f["phase"] == p["id"] and f["status"] in UNRESOLVED]
        if left:
            raise GateError(f"Unresolved findings in phase {p['id']}: {', '.join(left)}.")
        p["status"] = "awaiting-approval"
        fs = [f for f in state["findings"] if f["phase"] == p["id"]]
        body = (args.summary or "") + f"\n\nReviews passed. Findings this phase: {len(fs)} " \
            f"({sum(f['status'] == 'fixed' for f in fs)} fixed, {sum(f['status'] == 'accepted' for f in fs)} accepted)."
        q = add_question(state, "validation", f"Approve phase {p['id']}: {p['title']}?", body.strip(),
                         [APPROVE, REQUEST_CHANGES], {"type": "phase-approval", "phase": p["id"]},
                         {"stage": "implementation", "phase": p["id"]}, allow_free=False,
                         option_help=APPROVAL_HELP)
    print(f"{q['id']} asks the user to approve phase {args.id}. `wait {q['id']}`.")


def cmd_review_add(args, store):
    pid = parse_pid(args.phase)
    report = Path(args.report)
    if not report.exists():
        raise GateError(f"Report file {report} not found.")
    if (pid == "spec") != (args.type == SPEC_REVIEW):
        raise GateError("The specification review is `--phase spec --type specification`; "
                        "phases and the final review take functional and security reviews.")
    with store.edit() as state:
        extra = {}
        if pid == "spec":
            if state["stage"] != "specification":
                raise GateError("The specification review happens in the specification stage.")
            incomplete = {k: todo_lines(store, k) for k in SPECS if todo_lines(store, k)}
            if incomplete:
                raise GateError(f"The specification is not complete (TODOs in {', '.join(incomplete)}). "
                                "It is reviewed once everything is written.")
            if not state["phases"]:
                raise GateError("The phase plan is part of the specification: add phases first.")
            talks = [q["id"] for q in state["questions"] if q["status"] == "open" and q["kind"] != "validation"]
            if talks:
                raise GateError(f"Questions still open ({', '.join(talks)}): the specification is not final yet.")
            extra["specs_hash"] = specs_hash(state, store)
        elif pid == "final":
            if state["stage"] != "final-review":
                raise GateError("Final reviews are recorded in the final-review stage (`final start`).")
        else:
            p = phase(state, pid)
            if p["status"] != "in-progress":
                raise GateError(f"Phase {pid} is {p['status']}; reviews are recorded while it is in-progress.")
        rid = next_id(state, "R")
        slug = pid if pid in ("spec", "final") else f"phase-{pid}"
        dest = store.dir / "reviews" / f"{rid}-{slug}-{args.type}.md"
        dest.write_text(report.read_text())
        state["reviews"].append({
            "id": rid, "phase": pid, "type": args.type, "verdict": args.verdict,
            "summary": args.summary, "reviewer": args.reviewer, "agent": actor(state),
            "report": str(dest.relative_to(store.root)),
            "at": now(), **extra,
        })
        log(state, f"{rid}: {args.type} review of {where(pid)}: {args.verdict}.", "review")
    print(f"{rid} recorded ({args.verdict}). Record each finding with `finding add`.")


def cmd_finding_add(args, store):
    pid = parse_pid(args.phase)
    if (pid == "spec") != (args.review == SPEC_REVIEW):
        raise GateError("Specification findings are `--phase spec --review specification`.")
    if args.severity in BLOCKING and not (args.scenario and args.recommendation):
        raise GateError(f"A {args.severity} finding blocks the project and the user must decide on it: give "
                        "--scenario (what an input or user does, and what goes wrong) and --recommendation "
                        "(the fix you propose).")
    with store.edit() as state:
        if pid not in ("spec", "final"):
            phase(state, pid)
        fid = next_id(state, "F")
        f = {"id": fid, "phase": pid, "review": args.review, "severity": args.severity,
             "title": args.title, "detail": args.detail or "", "location": args.location or "",
             "scenario": args.scenario or "", "recommendation": args.recommendation or "",
             "status": "open", "resolution": "", "at": now(), "resolved_at": None,
             "found_by": actor(state)}
        state["findings"].append(f)
        log(state, f"{fid} ({args.severity}, {args.review}): {args.title}", "finding")
        q = None
        if args.severity in BLOCKING:
            q = add_question(state, "decision", f"{fid}: {args.title}", triage_body(state, f),
                             [TRIAGE_FIX, TRIAGE_REDESIGN, TRIAGE_ACCEPT, TRIAGE_STOP],
                             {"type": "finding-triage", "finding": fid},
                             {"stage": state["stage"], "phase": pid, "finding": fid,
                              "spec": args.spec or ("security" if args.review == "security" else None)},
                             option_help=TRIAGE_HELP)
    if q:
        print(f"{fid} recorded. BLOCKING: development stops. {q['id']} asks the user to decide. "
              f"Stop writing code and `wait {q['id']}`.")
    else:
        print(f"{fid} recorded.")


def triage_body(state, f):
    """The triage question: enough to decide without opening the code or the report."""
    meaning = SEVERITY_MEANING.get(f["review"], {}).get(f["severity"])
    review = latest_review(state, f["phase"], f["review"])
    source = (f"the {f['review']} review of {where(f['phase'])}" if f["review"] != SPEC_REVIEW
              else "the specification review")
    out = [f"**{f['severity'].upper()}** finding from {source}. Development is stopped until you decide."]
    if meaning:
        out.append(f"_{f['severity'].capitalize()} means: {meaning}_")
    if f["detail"]:
        out.append(f"### What is wrong\n\n{f['detail']}")
    out.append(f"### How it goes wrong\n\n{f['scenario']}")
    if f["location"]:
        out.append(f"### Where\n\n`{f['location']}`")
    out.append(f"### Proposed fix\n\n{f['recommendation']}")
    if review:
        out.append(f"Full review report: `{review['report']}` ({review['id']}).")
    out.append("Not sure which option fits? Use **Talk about this** to ask the agent first; "
               "development stays stopped meanwhile.")
    return "\n\n".join(out)


def cmd_finding_resolve(args, store):
    with store.edit() as state:
        f = next((f for f in state["findings"] if f["id"] == args.id), None)
        if not f:
            raise GateError(f"No finding {args.id}.")
        if f["status"] not in UNRESOLVED:
            raise GateError(f"{f['id']} is already {f['status']}.")
        if f["severity"] in BLOCKING:
            if args.status != "fixed":
                raise GateError(f"{f['id']} is {f['severity']}: only the user can accept it, through its triage question.")
            if any(q["status"] == "open" and (q.get("effect") or {}).get("finding") == f["id"] for q in state["questions"]):
                raise GateError(f"The user has not decided on {f['id']} yet. Wait for the triage answer.")
        f.update(status=args.status, resolution=args.note, resolved_at=now())
        log(state, f"{f['id']} {args.status}: {args.note}", "finding")
    print(f"{args.id} {args.status}.")


def cmd_ask(args, store):
    with store.edit() as state:
        ctx = {"stage": state["stage"]}
        if args.spec:
            ctx["spec"] = args.spec
        if args.phase:
            ctx["phase"] = parse_pid(args.phase)
        body = args.body
        if args.body_file:
            body = Path(args.body_file).read_text()
        options, helps = parse_options(args.option)
        q = add_question(state, args.kind, args.title, body or "", options, None, ctx,
                         allow_free=not args.no_free, option_help=helps)
    print(q["id"])


def cmd_reply(args, store):
    """The agent's side of a "Talk about this" thread; can also clarify the question itself."""
    text = Path(args.message_file).read_text() if args.message_file else args.message
    if not (text or "").strip():
        raise GateError("Give --message or --message-file.")
    with store.edit() as state:
        q = get_question(state, args.id)
        if q["status"] != "open":
            raise GateError(f"{q['id']} is no longer open.")
        q.setdefault("thread", []).append({"by": actor(state) or ORCHESTRATOR, "text": text.strip(), "at": now()})
        changed = []
        if args.body or args.body_file:
            q["body"] = Path(args.body_file).read_text() if args.body_file else args.body
            changed.append("question text")
        if args.option:
            if q.get("effect"):
                raise GateError(f"{q['id']}'s options drive the framework and cannot be changed; "
                                "explain them in the message instead.")
            q["options"], q["option_help"] = parse_options(args.option)
            changed.append("options")
        log(state, f"{q['id']}: agent replied" + (f" and revised the {' and '.join(changed)}." if changed else "."),
            "question")
    print(f"Reply added to {args.id}; it is still open. `wait {args.id}` for the answer or the next message.")


def cmd_wait(args, store):
    """Block until the questions are answered in the console.

    Meant to run as a background command: it exits, and so wakes the agent, as soon as the user
    answers. With no ids it watches every open question and returns on the first answer.
    """
    deadline = time.time() + args.timeout if args.timeout else None
    ids = args.ids or [q["id"] for q in store.read()["questions"] if q["status"] == "open"]
    first = args.any or not args.ids
    if not ids:
        print(f"No open questions.\nNext: {next_action(store.read(), store)}")
        return
    while True:
        state = store.read()
        qs = {q["id"]: q for q in state["questions"]}
        missing = [i for i in ids if i not in qs]
        if missing:
            raise GateError(f"No question {', '.join(missing)}.")
        done = [i for i in ids if qs[i]["status"] != "open"]
        pending = [i for i in ids if qs[i]["status"] == "open"]
        # A message in a "Talk about this" thread always wakes the agent: the user is waiting on it.
        talk = [i for i in ids if discussing(qs[i])]
        if talk or (done and (first or not pending)):
            out = [{"id": i, "title": qs[i]["title"], **(qs[i]["answer"] or {"withdrawn": True})} for i in done]
            out += [{"id": i, "title": qs[i]["title"], "status": "discussing", "thread": qs[i]["thread"]}
                    for i in talk]
            print(json.dumps(out if len(out) > 1 else out[0], indent=2))
            if talk:
                print(f"The user chose \"Talk about this\" on {', '.join(talk)}: it is NOT answered. Read the "
                      "thread, answer them with `reply <id> --message ...` (add --body/--option to clarify the "
                      "question itself), then `wait` again.")
            if any(qs[i]["status"] == "superseded" for i in done):
                print("A question was withdrawn because what it asked about changed. Do not treat it as answered.")
            if pending:
                print(f"Still open: {', '.join(pending)}. Act on the answers above, then wait again.")
            print(f"Next: {next_action(state, store)}")
            return
        if deadline and time.time() >= deadline:
            print(f"Still waiting on {', '.join(pending)}. Run `wait` again; do not proceed without the answer, "
                  "and do not ask for it in the chat.")
            sys.exit(3)
        time.sleep(2)


def cmd_models(args, store):
    """Models are machine-wide, per kind of project; this shows the current project's kind."""
    kind = store.read()["template"] if store else SOFTWARE
    if args.set:
        roles = {}
        for item in args.set:
            role, _, model = item.partition("=")
            roles[role.strip()] = model.strip()
        settings_update({kind: roles})
    models = settings_read()["models"][kind]
    roles = KINDS[kind]["roles"]
    if args.json:
        print(json.dumps({r: {"model": a, **MODELS[a]} for r, a in models.items()}, indent=2))
        return
    print(f"Models for {KINDS[kind]['label'].lower()} projects (console Settings page):")
    for r, a in models.items():
        print(f"  {r:<24} {a:<7} {MODELS[a]['name']:<11} {roles[r][0]}")
    print("\nPass the alias (second column) as the Agent tool's `model` when you spawn that role. "
          "The session itself runs the first role's model: if it differs, ask the user to `/model <alias>`.")


def cmd_inbox(args, store):
    """For CLIs without hooks: read (and mark delivered) the messages the user sent from the console."""
    with store.edit() as state:
        msg = take_messages(state)
    print(msg or "No new messages from the console.")


def cmd_agent_start(args, store):
    with store.edit() as state:
        if state["template"] != SOFTWARE:
            raise GateError("A single-agent task has one agent; helpers you spawn are not tracked.")
        if args.role == "orchestrator":
            raise GateError("The orchestrator is A-1 and always exists.")
        a = new_agent(state, args.role, args.task, parse_pid(args.phase) if args.phase else None)
    print(f"{a['id']} {a['name']}: {ROLES[a['role']][0].lower()} on {MODELS[a['model']]['name']} "
          f"(Agent tool model: {a['model']}).\n"
          f"Tell the subagent its name is {a['name']} and to run every lacarta command as "
          f"`python3 {Path(__file__).resolve()} --agent {a['id']} <command>`.\n"
          f"When it returns: `agent done {a['id']} --summary \"...\"` (or `--failed`).")


def cmd_agent_update(args, store):
    with store.edit() as state:
        a = get_agent(state, args.id)
        if a["status"] in ("done", "failed"):
            raise GateError(f"{a['name']} is {a['status']}.")
        if args.task:
            a["task"] = args.task
        if args.status:
            a["status"] = args.status
        a["updated_at"] = now()
        log(state, f"{a['name']}: {a['status']}, {a['task']}", "agent")
    print(f"{args.id} updated.")


def cmd_agent_done(args, store):
    with store.edit() as state:
        a = get_agent(state, args.id)
        if a["id"] == ORCHESTRATOR:
            raise GateError("The orchestrator finishes with the project.")
        if a["status"] in ("done", "failed"):
            raise GateError(f"{a['name']} is already {a['status']}.")
        a.update(status="failed" if args.failed else "done", summary=args.summary or "",
                 finished_at=now(), updated_at=now())
        log(state, f"{a['name']} {a['status']}" + (f": {args.summary}" if args.summary else "."), "agent")
    print(f"{args.id} {'failed' if args.failed else 'done'}.")


def cmd_agent_rename(args, store):
    with store.edit() as state:
        a = get_agent(state, args.id)
        name = args.name.strip()
        if not name or len(name) > 40:
            raise GateError("A name is 1 to 40 characters.")
        if any(x["name"] == name and x["id"] != a["id"] for x in state["agents"]):
            raise GateError(f"{name} is taken.")
        old, a["name"] = a["name"], name
        log(state, f"{old} is now called {name}.", "agent")
    print(f"{args.id} is {name}.")


def cmd_agent_list(args, store):
    state = store.read()
    for a in state["agents"]:
        print(f"  {a['id']:<5} {a['name']:<10} {a['role']:<24} {a['model']:<7} "
              f"{agent_status(state, a):<8} {a['task']}")


def cmd_decisions(args, store):
    state = store.read()
    qs = {q["id"]: q for q in state["questions"]}
    print(f"# Decision log: {state['project']['name']}\n")
    if not state["decisions"]:
        print("No decisions recorded.")
    for d in state["decisions"]:
        about = ", ".join(x for x in [f"spec: {d['spec']}" if d.get("spec") else "",
                                       where(d["phase"]) if d.get("phase") is not None else ""] if x)
        print(f"## {d['id']}: {d['title']}\n")
        print(f"- Decided by: {d['by']}, {d['at']}" + (f" ({about})" if about else ""))
        q = qs.get(d.get("source") or "")
        if q and q["body"]:
            print("- Question asked:\n\n" + "\n".join("  > " + line for line in q["body"].splitlines()))
            if q["options"]:
                print(f"\n  Options: {' / '.join(q['options'])}")
        if q and q.get("thread"):
            print("- Discussed before answering:")
            for m in q["thread"]:
                who = "User" if m["by"] == "user" else m["by"]
                print(f"\n  {who}: " + "\n  ".join(m["text"].splitlines()))
        print(f"- Decision: {d['decision']}")
        if d.get("rationale"):
            print(f"- Rationale or comment: {d['rationale']}")
        print()


def cmd_answer(args, store):
    with store.edit() as state:
        answer_question(state, args.id, args.choice or "", args.comment or "", by="user (terminal)")
    print(f"{args.id} answered on the user's behalf; the console shows it as given in the terminal.")


def cmd_decide(args, store):
    with store.edit() as state:
        did = next_id(state, "D")
        state["decisions"].append({
            "id": did, "title": args.title, "decision": args.decision, "rationale": args.rationale or "",
            "source": None, "by": "agent", "agent": actor(state), "spec": args.spec,
            "phase": ("final" if args.phase == "final" else int(args.phase)) if args.phase else None,
            "at": now(),
        })
        log(state, f"{did} recorded by agent: {args.title}", "decision")
    print(f"{did} recorded.")


def cmd_final_start(args, store):
    with store.edit() as state:
        reason = blocked_reason(state)
        if reason:
            raise GateError(f"Blocked. {reason}")
        if state["stage"] != "implementation":
            raise GateError(f"The final review follows implementation (now: {state['stage']}).")
        left = [p["id"] for p in state["phases"] if p["status"] != "done"]
        if left or not state["phases"]:
            raise GateError(f"Phases not done: {left or 'no phases'}.")
        state["stage"] = "final-review"
        log(state, "Final review started.", "stage")
    print("Stage: final-review. Spawn fresh reviewers as described in references/final-review.md.")


def cmd_final_complete(args, store):
    with store.edit() as state:
        reason = blocked_reason(state)
        if reason:
            raise GateError(f"Blocked. {reason}")
        if state["stage"] != "final-review":
            raise GateError(f"Not in final review (now: {state['stage']}).")
        for t in REVIEW_TYPES:
            r = latest_review(state, "final", t)
            if not r or r["verdict"] != "pass":
                raise GateError(f"The final {t} review has not passed.")
        left = [f["id"] for f in state["findings"] if f["phase"] == "final" and f["status"] in UNRESOLVED]
        if left:
            raise GateError(f"Unresolved final findings: {', '.join(left)}.")
        q = add_question(state, "validation", "Approve the final review and close the project?",
                         args.summary or "Both final reviews passed.", [APPROVE, REQUEST_CHANGES],
                         {"type": "final-approval"}, {"stage": "final-review", "phase": "final"},
                         allow_free=False, option_help=APPROVAL_HELP)
    print(f"{q['id']} asks for final approval. `wait {q['id']}`.")


# ------------------------------------------------------------------ server

LOOPBACK = {"127.0.0.1", "localhost", "::1"}
VENDOR = {"/vendor/marked.min.js", "/vendor/purify.min.js", "/vendor/mermaid.min.js"}
WILDCARD = {"0.0.0.0", "::", ""}


def url_host(host):
    return f"[{host}]" if ":" in host else host


def catalog():
    return {"models": MODELS, "planned": PLANNED_KINDS,
            "kinds": {k: {"label": v["label"], "about": v["about"],
                          "roles": {r: {"label": x[0], "stage": x[1], "about": x[2], "default": x[3]}
                                    for r, x in v["roles"].items()}} for k, v in KINDS.items()}}


def project_summary(pid, entry):
    """What the home page and the notifications need, cheap enough to compute for every project."""
    base = {"id": pid, "template": entry["template"], "name": entry.get("name", pid),
            "where": entry.get("root") or entry.get("cwd"), "archived": entry.get("archived", False)}
    try:
        state = project_store(pid).read()
    except (OSError, ValueError, GateError) as e:
        return {**base, "error": f"State unreadable: {e}"}
    a = get_agent(state, ORCHESTRATOR)
    names = {x["id"]: x["name"] for x in state["agents"]}
    qs = [{"id": q["id"], "title": q["title"], "asked_by": names.get(q.get("asked_by", ORCHESTRATOR), ""),
           "triage": (q.get("effect") or {}).get("type") == "finding-triage",
           "replies": sum(m["by"] != "user" for m in q.get("thread") or [])}
          for q in state["questions"] if q["status"] == "open"]
    return {**base, "name": state["project"]["name"], "stage": state["stage"],
            "status": agent_status(state, a), "agent": {k: a.get(k) for k in ("id", "name", "avatar")},
            "session": bool(a.get("session")) and not a["session"].get("ended_at"),
            "questions": qs, "attention": state.get("attention"), "listening": listening(state),
            "listening_since": (state.get("listening") or {}).get("since") if listening(state) else None,
            "blocked": blocked_reason(state) if state["template"] == SOFTWARE else None,
            "updated_at": state["activity"][-1]["at"] if state["activity"] else state["project"]["created_at"],
            "moved_to": state.get("moved_to")}


def project_payload(pid):
    store = project_store(pid)
    state = store.read()
    out = {"id": pid, "state": state, "derived": derived(state, store)}
    if state["template"] == SOFTWARE:
        out["specs"] = {k: (store.dir / "specs" / f"{k}.md").read_text() for k in SPECS}
        out["reports"] = {}
        for r in state["reviews"]:
            p = (store.root / r["report"]).resolve()
            if p.is_relative_to(store.dir.resolve()) and p.exists():
                out["reports"][r["id"]] = p.read_text()
    sess = get_agent(state, ORCHESTRATOR).get("session") or {}
    out["chat"] = read_chat(sess.get("transcript"))
    return out


def make_handler(token, port, extra_hosts=(), require_key=False, check_host=True):
    """Build the request handler.

    On loopback the page is open and only changes need the token. Bound to any other
    address, every request but the vendored scripts needs the token too (from `?key=` on
    the page URL, then the X-Lacarta-Token header), since anyone on the network can reach it.
    """
    allowed_hosts = {f"{url_host(h)}:{port}" for h in {*LOOPBACK, *extra_hosts}}

    class Handler(http.server.BaseHTTPRequestHandler):
        server_version = "lacarta"

        def log_message(self, *a):
            pass

        def _send(self, code, body, ctype="application/json", cache=False):
            data = body if isinstance(body, bytes) else body.encode()
            self.send_response(code)
            self.send_header("Content-Type", ctype + "; charset=utf-8" if not ctype.endswith("javascript") else ctype)
            self.send_header("Content-Length", str(len(data)))
            # Vendored libraries never change for a given skill release; everything else is live state.
            self.send_header("Cache-Control", "private, max-age=86400" if cache else "no-store")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.send_header("X-Frame-Options", "DENY")
            self.send_header("Referrer-Policy", "no-referrer")
            self.send_header("Content-Security-Policy",
                             "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' "
                             "'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; "
                             "connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; "
                             "frame-ancestors 'none'")
            self.end_headers()
            self.wfile.write(data)

        def _error(self, code, text):
            self._send(code, json.dumps({"error": text}))

        def _host_ok(self):
            # Refuse DNS rebinding: only the host names we were started for are served.
            if check_host and self.headers.get("Host") not in allowed_hosts:
                self._error(403, "Host not allowed. Add it with `serve --allow-host`.")
                return False
            return True

        def _key_ok(self, presented):
            if require_key and not secrets.compare_digest(presented or "", token):
                self._error(403, "Missing or wrong key. Open the URL `serve` printed.")
                return False
            return True

        def _project(self, path, action):
            """/api/p/<id>/<action> -> project id, or None."""
            parts = path.split("/")
            if len(parts) == 5 and parts[:3] == ["", "api", "p"] and parts[4] == action:
                return parts[3] if parts[3] in hub_read()["projects"] else None
            return None

        def do_GET(self):
            if not self._host_ok():
                return
            path, _, query = self.path.partition("?")
            if path in VENDOR:
                return self._send(200, (UI_DIR / path.lstrip("/")).read_bytes(), "text/javascript", cache=True)
            if path == "/":
                if not self._key_ok(urllib.parse.parse_qs(query).get("key", [""])[0]):
                    return
                html = (UI_DIR / "index.html").read_text().replace("__LACARTA_TOKEN__", token)
                return self._send(200, html, "text/html")
            if not self._key_ok(self.headers.get("X-Lacarta-Token")):
                return
            if path == "/api/hub":
                hub = hub_read()
                return self._send(200, json.dumps({
                    "projects": [project_summary(pid, e) for pid, e in hub["projects"].items()],
                    "settings": settings_read(), "catalog": catalog()}))
            pid = self._project(path, "state")
            if pid:
                try:
                    return self._send(200, json.dumps(project_payload(pid)))
                except (OSError, ValueError, GateError) as e:
                    return self._error(500, f"Cannot read this project: {e}")
            self._error(404, "Not found.")

        def do_POST(self):
            if not self._host_ok():
                return
            # Changes must come from the console page itself (same origin as the Host served).
            if self.headers.get("Origin") != f"http://{self.headers.get('Host')}":
                return self._error(403, "Origin not allowed.")
            if not secrets.compare_digest(self.headers.get("X-Lacarta-Token", ""), token):
                return self._error(403, "Missing or wrong token. Reload the page.")
            if not self.headers.get("Content-Type", "").startswith("application/json"):
                return self._error(415, "JSON only.")
            length = int(self.headers.get("Content-Length", 0))
            if length > 64_000:
                return self._error(413, "Too long.")
            try:
                body = json.loads(self.rfile.read(length))
                if self.path == "/api/settings":
                    models = body.get("models")
                    if models is not None and not isinstance(models, dict):
                        raise ValueError
                    s = settings_update({str(k): {str(r): str(m) for r, m in v.items()}
                                         for k, v in (models or {}).items()}, body.get("remote_control"))
                    return self._send(200, json.dumps({"ok": True, "settings": s}))
                for action in ("answer", "discuss", "message", "remote", "archive"):
                    pid = self._project(self.path, action)
                    if pid:
                        break
                else:
                    return self._error(404, "Not found.")
                if action == "archive":
                    with hub_edit() as hub:
                        hub["projects"][pid]["archived"] = bool(body.get("archived"))
                    return self._send(200, json.dumps({"ok": True}))
                with project_store(pid).edit() as state:
                    text = str(body.get("message", ""))[:20_000]
                    if action == "answer":
                        out = answer_question(state, str(body.get("id", "")), str(body.get("choice", "")),
                                              str(body.get("comment", ""))[:20_000])
                    elif action == "discuss":
                        out = discuss_question(state, str(body.get("id", "")), text)
                    elif action == "message":
                        out = add_message(state, text)
                    else:
                        on = body.get("on")
                        state["remote"] = None if on is None else bool(on)
                        log(state, f"Remote control {'on' if remote_on(state) else 'off'}.", "chat")
                        out = {"remote": remote_on(state)}
                self._send(200, json.dumps({"ok": True, "result": out}))
            except GateError as e:
                self._error(409, str(e))
            except (ValueError, KeyError, AttributeError, TypeError):
                self._error(400, "Malformed request.")

    return Handler


def cmd_serve(args, store=None):
    global ACTOR
    info = daemon_info()
    if info:
        print(f"lacarta console already running: {info['open_url']}")
        return
    ACTOR = None
    token = secrets.token_urlsafe(24)
    host = args.host
    exposed = host not in LOOPBACK
    wildcard = host in WILDCARD
    extra = set(args.allow_host or []) | ({host} if exposed and not wildcard else set())
    # A wildcard bind answers on every interface, so the Host check needs the names to expect;
    # without them the key alone guards the console.
    check_host = not wildcard or bool(args.allow_host)
    handler = make_handler(token, args.port, extra, require_key=exposed, check_host=check_host)

    class Server(http.server.ThreadingHTTPServer):
        address_family = socket.AF_INET6 if ":" in host else socket.AF_INET
        daemon_threads = True

    srv = Server((host, args.port), handler)
    shown = "<this-machine's-address>" if wildcard and not args.allow_host else \
        url_host(sorted(args.allow_host)[0] if wildcard else host)
    url = f"http://{shown}:{args.port}/"
    open_url = url + (f"?key={token}" if exposed else "")
    HOME.mkdir(parents=True, exist_ok=True)
    f = HOME / "daemon.json"
    f.write_text(json.dumps({"url": url, "open_url": open_url, "pid": os.getpid(), "started_at": now()}))
    f.chmod(0o600)      # it can hold the key
    print(f"lacarta console: {open_url}", flush=True)
    if exposed:
        print("Reachable from the network. Keep the key private: anyone holding the URL can answer "
              "for you. Traffic is plain HTTP; prefer a trusted network or an SSH tunnel.", flush=True)
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        with contextlib.suppress(OSError, ValueError):
            if json.loads(f.read_text()).get("pid") == os.getpid():
                f.unlink()


# --------------------------------------------------------------------- cli

def build_parser():
    ap = argparse.ArgumentParser(prog="lacarta", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", help="Software project root (default: nearest folder with .lacarta/).")
    ap.add_argument("--project", help="Project id from the console (default: the folder's .lacarta/, else "
                                      "$LACARTA_PROJECT, which a registered session sets).")
    ap.add_argument("--agent", help="Your agent id from `agent start` (default: $LACARTA_AGENT, else the "
                                    "orchestrator A-1). Goes before the command: `lacarta.py --agent A-3 ask ...`.")
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("init", help="Create .lacarta/ with spec templates.")
    p.add_argument("--name", help="Project name (default: the folder's name).")
    p.add_argument("--summary")

    p = sub.add_parser("status", help="Stage, gates, open items and the next expected action.")
    p.add_argument("--json", action="store_true")

    p = sub.add_parser("serve", help="Run the console for every project on this machine (loopback by default). "
                                     "Sessions start it on their own; this prints its URL if it already runs.")
    p.add_argument("--port", type=int, default=4747)
    p.add_argument("--host", default="127.0.0.1",
                   help="Address to bind. Anything but loopback requires the key printed in the URL.")
    p.add_argument("--allow-host", action="append",
                   help="Extra host name or IP the browser will use (LAN name, VPN IP); repeat.")

    p = sub.add_parser("install", help="Add the hooks that register every Claude Code session in the console.")
    p.add_argument("--settings", help="Claude Code settings file (default: ~/.claude/settings.json).")
    p.add_argument("--uninstall", action="store_true", help="Remove the hooks instead.")

    p = sub.add_parser("hook", help="Called by the CLI's hooks, with its JSON on stdin.")
    p.add_argument("event", choices=list(HOOKS))

    sub.add_parser("inbox", help="Read the messages the user sent from the console (CLIs without hooks).")

    spec = sub.add_parser("spec").add_subparsers(dest="sub", required=True)
    p = spec.add_parser("submit", help="Ask the user to approve one spec.")
    p.add_argument("kind", choices=list(SPECS))
    p.add_argument("--note", help="What changed or what to look at (markdown).")

    plan = sub.add_parser("plan").add_subparsers(dest="sub", required=True)
    plan.add_parser("submit", help="Ask the user to approve the phase plan.")
    p = plan.add_parser("reopen", help="Reopen the plan after a user decision changed it.")
    p.add_argument("--reason", required=True)

    ph = sub.add_parser("phase").add_subparsers(dest="sub", required=True)
    p = ph.add_parser("add", help="Append a phase to the plan.")
    p.add_argument("--title", required=True)
    p.add_argument("--goal", required=True, help="What is usable at the end of the phase.")
    p.add_argument("--scope", action="append", help="One deliverable; repeat.")
    p.add_argument("--covers", action="append", help="Requirement IDs covered (FR-1, SR-2); repeat.")
    p = ph.add_parser("start")
    p.add_argument("id", type=int)
    p = ph.add_parser("complete", help="Check gates and ask the user to approve the phase.")
    p.add_argument("id", type=int)
    p.add_argument("--summary", help="What was delivered (markdown).")

    p = sub.add_parser("review").add_subparsers(dest="sub", required=True).add_parser(
        "add", help="Record a review report: specification, functional or security.")
    p.add_argument("--phase", required=True, help="Phase number, 'spec' or 'final'.")
    p.add_argument("--type", required=True, choices=ALL_REVIEW_TYPES)
    p.add_argument("--verdict", required=True, choices=["pass", "fail"])
    p.add_argument("--report", required=True, help="Markdown report file to store.")
    p.add_argument("--summary", required=True)
    p.add_argument("--reviewer", default="agent", help="Who reviewed, e.g. 'subagent', 'fresh-agent'.")

    fd = sub.add_parser("finding").add_subparsers(dest="sub", required=True)
    p = fd.add_parser("add", help="Record a finding. High/critical blocks the project.")
    p.add_argument("--phase", required=True, help="Phase number, 'spec' or 'final'.")
    p.add_argument("--review", required=True, choices=ALL_REVIEW_TYPES)
    p.add_argument("--severity", required=True, choices=SEVERITIES)
    p.add_argument("--title", required=True)
    p.add_argument("--detail")
    p.add_argument("--location", help="file:line, component, or spec section.")
    p.add_argument("--scenario", help="What an input or user does and what goes wrong. Required for high/critical.")
    p.add_argument("--recommendation", help="The fix you propose. Required for high/critical.")
    p.add_argument("--spec", choices=list(SPECS), help="The spec a specification finding is about.")
    p = fd.add_parser("resolve")
    p.add_argument("id")
    p.add_argument("--status", required=True, choices=["fixed", "accepted", "wont-fix"])
    p.add_argument("--note", required=True, help="How it was fixed and verified, or why it is accepted.")

    p = sub.add_parser("ask", help="Ask the user a question in the console; prints its id.")
    p.add_argument("--kind", choices=["question", "decision", "validation"], default="question")
    p.add_argument("--title", required=True)
    p.add_argument("--body", help="Context in markdown.")
    p.add_argument("--body-file")
    p.add_argument("--option", action="append",
                   help="A choice, as \"Label|what choosing it means\"; repeat. Omit for free text.")
    p.add_argument("--no-free", action="store_true", help="Only the listed options are allowed.")
    p.add_argument("--spec", choices=list(SPECS))
    p.add_argument("--phase")

    p = sub.add_parser("reply", help="Answer the user in a question's \"Talk about this\" thread; "
                                     "the question stays open.")
    p.add_argument("id")
    p.add_argument("--message", help="Your reply, in markdown.")
    p.add_argument("--message-file")
    p.add_argument("--body", help="Replace the question text, when the discussion showed it was unclear.")
    p.add_argument("--body-file")
    p.add_argument("--option", action="append", help="Replace the options (\"Label|meaning\"); "
                                                     "not allowed on approvals and triage.")

    p = sub.add_parser("wait", help="Block until questions are answered in the console; prints the answers. "
                                    "Run it as a background command so the answer wakes you.")
    p.add_argument("ids", nargs="*", help="Questions to wait for (default: every open question, first answer wins).")
    p.add_argument("--any", action="store_true", help="Return as soon as one of the ids is answered.")
    p.add_argument("--timeout", type=int, default=0, help="Seconds before giving up (exit 3); 0 waits forever.")

    p = sub.add_parser("answer", help="Record an answer the user gave in the terminal, word for word.")
    p.add_argument("id")
    p.add_argument("--choice", help="One of the question's options.")
    p.add_argument("--comment", help="The user's words.")

    sub.add_parser("decisions", help="Print the decision log as markdown (for the spec reviewer).")

    ag = sub.add_parser("agent", help="Register and track the agents shown in the console.").add_subparsers(
        dest="sub", required=True)
    p = ag.add_parser("start", help="Register a subagent before spawning it; gives its id, name and model.")
    p.add_argument("--role", required=True, choices=[r for r in ROLES if r != "orchestrator"])
    p.add_argument("--task", required=True, help="One line: what it is doing.")
    p.add_argument("--phase", help="Phase number, 'spec' or 'final'.")
    p = ag.add_parser("update", help="Change an agent's current task or status.")
    p.add_argument("id")
    p.add_argument("--task")
    p.add_argument("--status", choices=["working", "idle"])
    p = ag.add_parser("done", help="Mark a subagent finished.")
    p.add_argument("id")
    p.add_argument("--summary")
    p.add_argument("--failed", action="store_true")
    p = ag.add_parser("rename", help="Rename an agent (when the user asks).")
    p.add_argument("id")
    p.add_argument("--name", required=True)
    ag.add_parser("list", help="All agents with their status.")

    p = sub.add_parser("models", help="Show the model for each role of this kind of project "
                                      "(read it before every spawn).")
    p.add_argument("--json", action="store_true")
    p.add_argument("--set", action="append", metavar="ROLE=MODEL",
                   help=f"Change a role's model as the user asked in the terminal. Models: {', '.join(MODELS)}.")

    p = sub.add_parser("decide", help="Log a decision you made yourself.")
    p.add_argument("--title", required=True)
    p.add_argument("--decision", required=True)
    p.add_argument("--rationale")
    p.add_argument("--spec", choices=list(SPECS))
    p.add_argument("--phase")

    fin = sub.add_parser("final").add_subparsers(dest="sub", required=True)
    fin.add_parser("start")
    p = fin.add_parser("complete")
    p.add_argument("--summary")
    return ap


COMMANDS = {
    ("status", None): cmd_status, ("serve", None): cmd_serve,
    ("spec", "submit"): cmd_spec_submit, ("plan", "submit"): cmd_plan_submit,
    ("plan", "reopen"): cmd_plan_reopen, ("phase", "add"): cmd_phase_add,
    ("phase", "start"): cmd_phase_start, ("phase", "complete"): cmd_phase_complete,
    ("review", "add"): cmd_review_add, ("finding", "add"): cmd_finding_add,
    ("finding", "resolve"): cmd_finding_resolve, ("ask", None): cmd_ask, ("wait", None): cmd_wait,
    ("reply", None): cmd_reply, ("answer", None): cmd_answer, ("decide", None): cmd_decide, ("decisions", None): cmd_decisions,
    ("models", None): cmd_models, ("inbox", None): cmd_inbox,
    ("agent", "start"): cmd_agent_start, ("agent", "update"): cmd_agent_update,
    ("agent", "done"): cmd_agent_done, ("agent", "rename"): cmd_agent_rename,
    ("agent", "list"): cmd_agent_list,
    ("final", "start"): cmd_final_start,
    ("final", "complete"): cmd_final_complete,
}


SOFTWARE_ONLY = {"spec", "plan", "phase", "review", "finding", "final"}


def resolve_store(args):
    if args.root:
        return Store(find_root(args.root))
    if args.project:
        return project_store(args.project)
    root = find_root()
    if root:
        # A project from before the console was machine-wide joins it on first use.
        if not any(p.get("root") == str(root) for p in hub_read()["projects"].values()):
            register_software(root, json.loads((root / STATE_DIR / "state.json").read_text())["project"]["name"])
        return Store(root)
    if os.environ.get("LACARTA_PROJECT"):
        return project_store(os.environ["LACARTA_PROJECT"])
    return None


def main(argv=None):
    global ACTOR
    args = build_parser().parse_args(argv)
    ACTOR = args.agent or os.environ.get("LACARTA_AGENT") or ORCHESTRATOR
    try:
        if args.cmd == "hook":
            return cmd_hook(args)
        if args.cmd in ("init", "install", "serve"):
            return {"init": cmd_init, "install": cmd_install, "serve": cmd_serve}[args.cmd](args)
        store = resolve_store(args)
        if args.cmd == "models":
            return cmd_models(args, store)
        if not store or not store.path.exists():
            raise GateError("No project here. `init` creates a software project in this folder; sessions "
                            "register on their own once `install` has added the hooks.")
        if args.cmd in SOFTWARE_ONLY and store.read()["template"] != SOFTWARE:
            raise GateError(f"`{args.cmd}` belongs to software projects; this is a single-agent task. "
                            "Run `init` to turn this folder into a software project.")
        COMMANDS[(args.cmd, getattr(args, "sub", None))](args, store)
    except GateError as e:
        print(f"Refused: {e}", file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
