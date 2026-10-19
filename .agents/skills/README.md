# Project skills

This directory is the one place project skills live. Each child directory is an
[Agent Skills](https://agentskills.io/specification) skill with a `SKILL.md`
whose frontmatter has a `name` (matching the directory) and a `description`.

```text
.agents/skills/       the skills themselves
.claude/skills -> ../.agents/skills
```

- Skills live in `.agents/skills`. `.claude/skills` is a symlink to it, tracked
  in git; the rest of `.claude/` is ignored.
- Shared skills are copies from `../skills`, listed with their source commit
  and content hash in `shared.sum`. `make skills-check` fails on a copy
  edited here; edit the source, commit it, then `make skills-sync`.
- Never copy a skill into a client directory. Copies drift.
- Keep `SKILL.md` free of client-only tool names and slash-command syntax.
- A skill is worth adding when a tool or a reviewer acts on it. Descriptions
  name the paths or situations that trigger the skill.

| Skill | Shared | Use it for |
|---|---|---|
| `project-docs` | yes | The document set, comments, where a fact belongs |
| `sound-patterns` | yes | Settling style questions against the Go stdlib |
| `go-conformance-suite` | yes | One test suite over several implementations of an interface |
