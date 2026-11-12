#!/usr/bin/env python3
"""lacarta: project state, gates and review console for the lacarta framework.

State lives in <project>/.lacarta/ (state.json, specs/, reviews/). Every command
reads and writes that folder under a file lock, and the gates refuse to move the
project forward while the framework's rules are not met. Standard library only.

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
import socket
import sys
import time
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
USE_MODELS = "Use these models"

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


class GateError(Exception):
    """A framework rule refuses the requested transition."""


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


# ---------------------------------------------------------------- storage

def find_root(explicit=None):
    if explicit:
        return Path(explicit).resolve()
    here = Path.cwd().resolve()
    for d in [here, *here.parents]:
        if (d / STATE_DIR / "state.json").exists():
            return d
    return None


class Store:
    def __init__(self, root):
        self.root = Path(root)
        self.dir = self.root / STATE_DIR
        self.path = self.dir / "state.json"

    @contextlib.contextmanager
    def _lock(self, exclusive):
        with open(self.dir / ".lock", "a+") as lf:
            if fcntl:
                fcntl.flock(lf, fcntl.LOCK_EX if exclusive else fcntl.LOCK_SH)
            yield

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
            tmp = self.path.with_suffix(".tmp")
            tmp.write_text(json.dumps(state, indent=2) + "\n")
            os.replace(tmp, self.path)


def default_models(confirmed):
    return {"confirmed": confirmed, "roles": {r: v[3] for r, v in ROLES.items()}}


def migrate(state):
    """Bring a state file written by an older version up to date."""
    if "models" not in state:
        # A project that predates model settings is already under way: keep it moving.
        state["models"] = default_models(confirmed=True)
    for r, v in ROLES.items():
        state["models"]["roles"].setdefault(r, v[3])
    if "agents" not in state:
        state["agents"] = []
        new_agent(state, "orchestrator", "Runs the framework and talks to you.", log_it=False)
    return state


# ------------------------------------------------------------------ agents

def new_agent(state, role, task, phase=None, log_it=True):
    used = {a["name"] for a in state["agents"]}
    n = len(state["agents"])
    name = next((x for x in AGENT_NAMES if x not in used), f"{AGENT_NAMES[n % len(AGENT_NAMES)]} {n // len(AGENT_NAMES) + 1}")
    a = {
        "id": f"A-{n + 1}", "name": name, "role": role, "model": state["models"]["roles"][role],
        "task": task, "phase": phase, "status": "working", "summary": "",
        "avatar": None,  # reserved for a custom profile picture; the console shows initials meanwhile
        "started_at": now(), "updated_at": now(), "finished_at": None,
    }
    state["agents"].append(a)
    if log_it:
        log(state, f"{name} started as {ROLES[role][0].lower()} on {MODELS[a['model']]['name']}: {task}", "agent")
    return a


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
    return a["status"]


def set_models(state, roles, by):
    """Apply {role: model} changes, logging each one as a decision."""
    for role, model in roles.items():
        if role not in ROLES:
            raise GateError(f"Unknown role {role}. Roles: {', '.join(ROLES)}.")
        if model not in MODELS:
            raise GateError(f"Unknown model {model}. Models: {', '.join(MODELS)}.")
        old = state["models"]["roles"][role]
        if old == model:
            continue
        state["models"]["roles"][role] = model
        state["decisions"].append({
            "id": next_id(state, "D"), "title": f"Model for {ROLES[role][0].lower()}",
            "decision": f"{MODELS[model]['name']} (was {MODELS[old]['name']})", "rationale": "",
            "source": None, "by": by, "spec": None, "phase": None, "at": now(),
        })
        log(state, f"{ROLES[role][0]} now uses {MODELS[model]['name']}.", "models")


def require_models(state):
    if not state["models"]["confirmed"]:
        q = open_question(state, {"type": "models-confirm"})
        raise GateError("The user has not confirmed the models yet" + (f" (`wait {q['id']}`)." if q else "."))


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
    triage = [q for q in state["questions"]
              if q["status"] == "open" and (q.get("effect") or {}).get("type") == "finding-triage"]
    if triage:
        return f"Blocked: wait for the user's decision on {triage[0]['id']} (`wait {triage[0]['id']}`). Do not write code."
    if blocking_findings(state):
        f = blocking_findings(state)[0]
        return (f"Blocked on {f['id']} ({f['status']}): fix it, re-run the {f['review']} review, "
                f"then `finding resolve {f['id']} --status fixed`. Nothing else advances.")
    waiting = [q for q in state["questions"] if q["status"] == "open"]
    if not state["models"]["confirmed"]:
        q = open_question(state, {"type": "models-confirm"})
        return (f"Wait for the user to set and confirm the models in the console (`wait {q['id']}`)."
                if q else "Models are not confirmed.")
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
    sev = {s: 0 for s in SEVERITIES}
    for f in state["findings"]:
        if f["status"] in UNRESOLVED:
            sev[f["severity"]] += 1
    return {
        "blocked": blocked_reason(state),
        "next": next_action(state, store),
        "spec_review": spec_review_status(state, store),
        "open_questions": sum(q["status"] == "open" for q in state["questions"]),
        "open_findings": sev,
        "specs_approved": sum(s["status"] == "approved" for s in state["specs"].values()),
        "phases_done": sum(p["status"] == "done" for p in state["phases"]),
        "agent_status": {a["id"]: agent_status(state, a) for a in state["agents"]},
    }


# --------------------------------------------------------------- questions

def add_question(state, kind, title, body="", options=None, effect=None, context=None, allow_free=True):
    if effect:
        existing = open_question(state, effect)
        if existing:
            return existing
    q = {
        "id": next_id(state, "Q"), "kind": kind, "title": title, "body": body,
        "options": options or [], "allow_free": allow_free, "context": context or {},
        "effect": effect, "status": "open", "asked_at": now(), "answer": None,
        "asked_by": actor(state) or ORCHESTRATOR,
    }
    state["questions"].append(q)
    log(state, f"{q['id']} asked: {title}", "question")
    return q


def answer_question(state, qid, choice, comment="", by="user"):
    q = next((q for q in state["questions"] if q["id"] == qid), None)
    if not q:
        raise GateError(f"No question {qid}.")
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
    elif t == "models-confirm":
        state["models"]["confirmed"] = True
        log(state, "Models confirmed: " + ", ".join(
            f"{ROLES[r][0]} {MODELS[m]['name']}" for r, m in state["models"]["roles"].items()) + ".", "models")
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
    (d / ".gitignore").write_text(".lock\nstate.tmp\nserver.json\n")
    state = {
        "version": 1,
        "project": {"name": args.name, "summary": args.summary or "", "created_at": now()},
        "stage": "specification",
        "specs": {k: {"title": v, "status": "draft", "revision": 0, "approved_at": None,
                      "approved_revision": None} for k, v in SPECS.items()},
        "plan": {"status": "draft", "approved_at": None},
        "phases": [], "reviews": [], "questions": [], "decisions": [], "findings": [],
        "activity": [], "counters": {}, "models": default_models(confirmed=False),
    }
    migrate(state)  # registers the orchestrator
    log(state, f"Project {args.name} initialised.", "stage")
    table = "\n".join(f"| {v[0]} | {MODELS[v[3]]['name']} |" for v in ROLES.values())
    q = add_question(
        state, "validation", "Choose the models for this project",
        "Each part of the framework runs on its own model. Defaults:\n\n| Role | Model |\n| --- | --- |\n"
        f"{table}\n\nChange them under [Models](#models), then confirm there or here. "
        "You can change them again at any time; agents read the current choice before each spawn.",
        [USE_MODELS], {"type": "models-confirm"}, {"stage": "specification"}, allow_free=False)
    (d / "state.json").write_text(json.dumps(state, indent=2) + "\n")
    print(f"Initialised {d}\nSpec templates: {d / 'specs'}\n"
          f"Next: start the console with `serve`, then `wait {q['id']}` while the user sets the models.")


def cmd_status(args, store):
    state = store.read()
    if args.json:
        print(json.dumps({**state, "derived": derived(state, store)}, indent=2))
        return
    d = derived(state, store)
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
            {"stage": "specification", "spec": args.kind}, allow_free=False)
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
                         {"stage": "specification"}, allow_free=False)
    print(f"{q['id']} asks the user to approve the plan. `wait {q['id']}`.")


def cmd_plan_reopen(args, store):
    with store.edit() as state:
        state["plan"]["status"] = "draft"
        log(state, f"Phase plan reopened: {args.reason}", "plan")
    print("Plan reopened. Edit phases, then `plan submit`.")


def cmd_phase_start(args, store):
    with store.edit() as state:
        require_models(state)
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
                         {"stage": "implementation", "phase": p["id"]}, allow_free=False)
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
        require_models(state)
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
    with store.edit() as state:
        if pid not in ("spec", "final"):
            phase(state, pid)
        fid = next_id(state, "F")
        f = {"id": fid, "phase": pid, "review": args.review, "severity": args.severity,
             "title": args.title, "detail": args.detail or "", "location": args.location or "",
             "status": "open", "resolution": "", "at": now(), "resolved_at": None,
             "found_by": actor(state)}
        state["findings"].append(f)
        log(state, f"{fid} ({args.severity}, {args.review}): {args.title}", "finding")
        q = None
        if args.severity in BLOCKING:
            body = (f"**{args.severity.upper()}** {args.review} finding in {where(pid)}.\n\n"
                    f"{args.detail or ''}\n\n" + (f"Location: `{args.location}`\n\n" if args.location else "") +
                    "Development is stopped until you decide.")
            q = add_question(state, "decision", f"{fid}: {args.title}", body,
                             [TRIAGE_FIX, TRIAGE_REDESIGN, TRIAGE_ACCEPT, TRIAGE_STOP],
                             {"type": "finding-triage", "finding": fid},
                             {"stage": state["stage"], "phase": pid, "finding": fid,
                              "spec": args.spec or ("security" if args.review == "security" else None)})
    if q:
        print(f"{fid} recorded. BLOCKING: development stops. {q['id']} asks the user to decide. "
              f"Stop writing code and `wait {q['id']}`.")
    else:
        print(f"{fid} recorded.")


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
        q = add_question(state, args.kind, args.title, body or "", args.option or [], None, ctx,
                         allow_free=not args.no_free)
    print(q["id"])


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
        if done and (first or not pending):
            out = [{"id": i, "title": qs[i]["title"], **(qs[i]["answer"] or {"withdrawn": True})} for i in done]
            print(json.dumps(out if len(out) > 1 else out[0], indent=2))
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
    if args.set:
        roles = {}
        for item in args.set:
            role, _, model = item.partition("=")
            roles[role.strip()] = model.strip()
        with store.edit() as state:
            set_models(state, roles, by="user (terminal)")
    state = store.read()
    m = state["models"]
    if args.json:
        print(json.dumps({r: {"model": a, **MODELS[a]} for r, a in m["roles"].items()}
                         | {"confirmed": m["confirmed"]}, indent=2))
        return
    print(f"Models ({'confirmed' if m['confirmed'] else 'NOT confirmed yet'}):")
    for r, a in m["roles"].items():
        print(f"  {r:<24} {a:<7} {MODELS[a]['name']:<11} {ROLES[r][0]}")
    print("\nPass the alias (second column) as the Agent tool's `model` when you spawn that role. "
          "The orchestrator is this session: if it runs another model, ask the user to `/model <alias>`.")


def cmd_agent_start(args, store):
    with store.edit() as state:
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
                         allow_free=False)
    print(f"{q['id']} asks for final approval. `wait {q['id']}`.")


# ------------------------------------------------------------------ server

LOOPBACK = {"127.0.0.1", "localhost", "::1"}
VENDOR = {"/vendor/marked.min.js", "/vendor/purify.min.js", "/vendor/mermaid.min.js"}
WILDCARD = {"0.0.0.0", "::", ""}


def url_host(host):
    return f"[{host}]" if ":" in host else host


def make_handler(store, token, port, extra_hosts=(), require_key=False, check_host=True):
    """Build the request handler.

    On loopback the page is open and only answers need the token. Bound to any other
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

        def _host_ok(self):
            # Refuse DNS rebinding: only the host names we were started for are served.
            if check_host and self.headers.get("Host") not in allowed_hosts:
                self._send(403, json.dumps({"error": "Host not allowed. Add it with `serve --allow-host`."}))
                return False
            return True

        def _key_ok(self, presented):
            if require_key and not secrets.compare_digest(presented or "", token):
                self._send(403, json.dumps({"error": "Missing or wrong key. Open the URL `serve` printed."}))
                return False
            return True

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
            if path == "/api/state":
                if not self._key_ok(self.headers.get("X-Lacarta-Token")):
                    return
                state = store.read()
                specs = {k: (store.dir / "specs" / f"{k}.md").read_text() for k in SPECS}
                reports = {}
                for r in state["reviews"]:
                    p = (store.root / r["report"]).resolve()
                    if p.is_relative_to(store.dir.resolve()) and p.exists():
                        reports[r["id"]] = p.read_text()
                catalog = {"models": MODELS, "roles": {r: {"label": v[0], "stage": v[1], "about": v[2],
                                                            "default": v[3]} for r, v in ROLES.items()}}
                return self._send(200, json.dumps({"state": state, "derived": derived(state, store),
                                                   "specs": specs, "reports": reports, "catalog": catalog}))
            self._send(404, json.dumps({"error": "Not found."}))

        def do_POST(self):
            if not self._host_ok():
                return
            # Answers must come from the console page itself (same origin as the Host served).
            if self.headers.get("Origin") != f"http://{self.headers.get('Host')}":
                return self._send(403, json.dumps({"error": "Origin not allowed."}))
            if not secrets.compare_digest(self.headers.get("X-Lacarta-Token", ""), token):
                return self._send(403, json.dumps({"error": "Missing or wrong token. Reload the page."}))
            if not self.headers.get("Content-Type", "").startswith("application/json"):
                return self._send(415, json.dumps({"error": "JSON only."}))
            if self.path not in ("/api/answer", "/api/models"):
                return self._send(404, json.dumps({"error": "Not found."}))
            length = int(self.headers.get("Content-Length", 0))
            if length > 64_000:
                return self._send(413, json.dumps({"error": "Answer too long."}))
            try:
                body = json.loads(self.rfile.read(length))
                if self.path == "/api/models":
                    with store.edit() as state:
                        roles = body.get("roles") or {}
                        if not isinstance(roles, dict):
                            raise ValueError
                        set_models(state, {str(k): str(v) for k, v in roles.items()}, by="user")
                        if body.get("confirm") and not state["models"]["confirmed"]:
                            q = open_question(state, {"type": "models-confirm"})
                            if q:
                                answer_question(state, q["id"], USE_MODELS)
                            else:
                                state["models"]["confirmed"] = True
                    return self._send(200, json.dumps({"ok": True, "models": state["models"]}))
                with store.edit() as state:
                    q = answer_question(state, str(body.get("id", "")), str(body.get("choice", "")),
                                        str(body.get("comment", ""))[:20_000])
                self._send(200, json.dumps({"ok": True, "question": q}))
            except GateError as e:
                self._send(409, json.dumps({"error": str(e)}))
            except (ValueError, KeyError):
                self._send(400, json.dumps({"error": "Malformed request."}))

    return Handler


def cmd_serve(args, store):
    global ACTOR
    ACTOR = None
    token = secrets.token_urlsafe(24)
    host = args.host
    exposed = host not in LOOPBACK
    wildcard = host in WILDCARD
    extra = set(args.allow_host or []) | ({host} if exposed and not wildcard else set())
    # A wildcard bind answers on every interface, so the Host check needs the names to expect;
    # without them the key alone guards the console.
    check_host = not wildcard or bool(args.allow_host)
    handler = make_handler(store, token, args.port, extra, require_key=exposed, check_host=check_host)

    class Server(http.server.ThreadingHTTPServer):
        address_family = socket.AF_INET6 if ":" in host else socket.AF_INET

    srv = Server((host, args.port), handler)
    shown = "<this-machine's-address>" if wildcard and not args.allow_host else \
        url_host(sorted(args.allow_host)[0] if wildcard else host)
    url = f"http://{shown}:{args.port}/" + (f"?key={token}" if exposed else "")
    (store.dir / "server.json").write_text(json.dumps({"url": url.split("?")[0], "pid": os.getpid(),
                                                        "started_at": now()}))
    print(f"lacarta console: {url}", flush=True)
    if exposed:
        print("Reachable from the network. Keep the key private: anyone holding the URL can answer "
              "for you. Traffic is plain HTTP; prefer a trusted network or an SSH tunnel.", flush=True)
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        (store.dir / "server.json").unlink(missing_ok=True)


# --------------------------------------------------------------------- cli

def build_parser():
    ap = argparse.ArgumentParser(prog="lacarta", description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--root", help="Project root (default: nearest folder with .lacarta/).")
    ap.add_argument("--agent", help="Your agent id from `agent start` (default: $LACARTA_AGENT, else the "
                                    "orchestrator A-1). Goes before the command: `lacarta.py --agent A-3 ask ...`.")
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("init", help="Create .lacarta/ with spec templates.")
    p.add_argument("--name", help="Project name (default: the folder's name).")
    p.add_argument("--summary")

    p = sub.add_parser("status", help="Stage, gates, open items and the next expected action.")
    p.add_argument("--json", action="store_true")

    p = sub.add_parser("serve", help="Run the web console (loopback by default).")
    p.add_argument("--port", type=int, default=4747)
    p.add_argument("--host", default="127.0.0.1",
                   help="Address to bind. Anything but loopback requires the key printed in the URL.")
    p.add_argument("--allow-host", action="append",
                   help="Extra host name or IP the browser will use (LAN name, VPN IP); repeat.")

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
    p.add_argument("--option", action="append", help="A choice; repeat. Omit for free text.")
    p.add_argument("--no-free", action="store_true", help="Only the listed options are allowed.")
    p.add_argument("--spec", choices=list(SPECS))
    p.add_argument("--phase")

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

    p = sub.add_parser("models", help="Show the model for each role (read it before every spawn).")
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
    ("answer", None): cmd_answer, ("decide", None): cmd_decide, ("decisions", None): cmd_decisions,
    ("models", None): cmd_models,
    ("agent", "start"): cmd_agent_start, ("agent", "update"): cmd_agent_update,
    ("agent", "done"): cmd_agent_done, ("agent", "rename"): cmd_agent_rename,
    ("agent", "list"): cmd_agent_list,
    ("final", "start"): cmd_final_start,
    ("final", "complete"): cmd_final_complete,
}


def main(argv=None):
    global ACTOR
    args = build_parser().parse_args(argv)
    ACTOR = args.agent or os.environ.get("LACARTA_AGENT") or ORCHESTRATOR
    try:
        if args.cmd == "init":
            return cmd_init(args)
        root = find_root(args.root)
        if not root or not (root / STATE_DIR / "state.json").exists():
            raise GateError("No .lacarta/ found here or above. Run `init` in the project root first.")
        COMMANDS[(args.cmd, getattr(args, "sub", None))](args, Store(root))
    except GateError as e:
        print(f"Refused: {e}", file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
