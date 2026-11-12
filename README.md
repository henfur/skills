# lacarta

An agent skill that runs every project through the same framework:

1. **Specification**: functional, architecture, tech stack, and security / threat model specs, plus a phase plan, each approved by you.
2. **Implementation**: one phase at a time, each closed by a functional review, a security review and your approval. A high or critical finding stops development until you decide.
3. **Final review**: two fresh agents (functional, security) with no memory of the build, judging only the repository.

A local web console shows the questions waiting for you, the specs, every decision, phases with their reviews, findings and the overall status.

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

Claude Code (personal skills):

```bash
ln -s "$PWD/lacarta" ~/.claude/skills/lacarta
```

Then, in any project: "let's build an invoice tracker", or "use lacarta for this".

## Console

`python3 lacarta/scripts/lacarta.py serve` serves the console at `http://127.0.0.1:4747/` for the nearest project with a `.lacarta/` folder. By default it binds to loopback, rejects foreign `Host` and `Origin` headers, and requires a per-run token for answers.

To reach it from another machine, bind another address: `serve --host 10.0.0.5` (or `--host 0.0.0.0 --allow-host my-box.lan`). The page and API then require the key printed in the URL (`?key=…`). Traffic is plain HTTP, so use a trusted network, or keep the default and tunnel with `ssh -L 4747:127.0.0.1:4747 host`.
