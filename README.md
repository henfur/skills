# lacarta

An agent skill that runs every project through the same framework:

1. **Specification**: functional, architecture, tech stack, and security / threat model specs, plus a phase plan, each approved by you.
2. **Implementation**: one phase at a time, each closed by a functional review, a security review and your approval. A high or critical finding stops development until you decide.
3. **Final review**: two fresh agents (functional, security) with no memory of the build, judging only the repository.

One web console per machine shows every agent session and project: the questions waiting for you, notifications, each session's chat (with remote control: write to an agent from the browser), and for software projects the specs, every decision, phases with their reviews, findings and the overall status.

Every Claude Code session registers itself through hooks. A session in a folder with a `.lacarta/` software project joins it; any other session appears as a **single-agent task** (overview, chat, questions, activity). More kinds of project (security audit, DevOps) will get their own views. Models are set once per kind of project on the console's Settings page.

## Layout

```
lacarta/
├── SKILL.md                  # the framework, for the agent
├── scripts/lacarta.py        # CLI + console server (Python 3.9+, stdlib only)
├── references/               # stage guides and reviewer prompts
└── assets/
    ├── templates/            # the four spec templates
    └── ui/                   # console page; vendored marked 12.0.2, DOMPurify 3.1.6, Mermaid 10.9.1
```

## Install

Claude Code (personal skills), then the hooks that register every session:

```bash
ln -s "$PWD/lacarta" ~/.claude/skills/lacarta
python3 lacarta/scripts/lacarta.py install      # adds hooks to ~/.claude/settings.json (backup kept); --uninstall removes them
```

New sessions then appear in the console, which the first session starts on its own. In any project: "let's build an invoice tracker", or "use lacarta for this".

## Console

One daemon serves every project at `http://127.0.0.1:4747/`. Sessions start it when they register; `python3 lacarta/scripts/lacarta.py serve` prints its URL, or starts it. Machine-wide state lives in `~/.lacarta/` (`$LACARTA_HOME`): the project registry, settings, single-agent tasks, and `hooks.log` if a hook fails. A software project's state stays in its own `.lacarta/` folder.

By default the console binds to loopback, rejects foreign `Host` and `Origin` headers, and requires a per-run token for every change. It only serves session transcripts from inside `~/.claude/`.

**Remote control.** Each session keeps `lacarta listen` running as a background command. When you write from the console, answer a question or ask about one, the listener returns and Claude Code wakes the agent, even when its turn had ended. The terminal stays usable the whole time. Turn it off machine-wide in Settings, or per project in its Chat.

To reach the console from another machine, start it on another address: `serve --host 10.0.0.5` (or `--host 0.0.0.0 --allow-host my-box.lan`). The page and API then require the key printed in the URL (`?key=…`). Traffic is plain HTTP, so use a trusted network, or keep the default and tunnel with `ssh -L 4747:127.0.0.1:4747 host`.
