# Sources

Each line says what the source contributes to the skill.
Quotes are as fetched; where a fetch tool paraphrased, it is marked.

## Agent-facing files

- https://agents.md — the AGENTS.md convention: "a README for agents";
  suggested sections (overview, build/test, style, testing, security, PR
  rules); nearest nested file wins; migration by rename plus symlink. No
  length limit of its own.
- https://code.claude.com/docs/en/memory — "target under 200 lines per
  CLAUDE.md file. Longer files consume more context and reduce adherence";
  `@path` imports load at launch and "don't reduce its context cost";
  AGENTS.md is read when no CLAUDE.md exists; contradictions are resolved
  arbitrarily; `/doctor prompt-audit` flags dead references.
- https://code.claude.com/docs/en/best-practices — "For each line, ask:
  'Would removing this cause Claude to make mistakes?' If not, cut it";
  "hooks are deterministic" while the file is advisory; skills for on-demand
  knowledge.
- https://www.humanlayer.dev/blog/writing-a-good-claude-md — under 300
  lines, shorter is better; "~150–200 instructions" is the ceiling a
  frontier model follows; pointers over copied code; "Never send an LLM to
  do a linter's job"; don't auto-generate the file.
- https://learn.chatgpt.com/docs/agent-configuration/agents-md — Codex
  concatenates AGENTS.md root-down, closer overrides, 32 KiB combined cap.
- https://agentskills.io/specification — `name` and `description` required;
  body under 500 lines; `references/` one level deep; `scripts/` for
  helpers; ~100 tokens of metadata at startup, body on trigger.
- https://martinfowler.com/articles/exploring-gen-ai/context-engineering-coding-agents.html
  — rules modularise, skills lazy-load, hooks are the deterministic
  exception; "illusion of control".
- https://arxiv.org/pdf/2602.11988 — "Evaluating AGENTS.md": generated
  context files lowered success and raised cost; human-written files should
  state minimal requirements.
- https://arxiv.org/abs/2511.12884 — "Agent READMEs": 2,303 context files;
  they evolve by small additions like config; test instructions in 76%,
  architecture in 68%.

## Documents and maintenance

- https://diataxis.fr/how-to-use-diataxis/ — "It certainly does not mean that
  you should create empty structures"; "Good structure develops from
  within"; a lens, not a template.
- https://google.github.io/styleguide/docguide/best_practices.html — "Change
  your documentation in the same CL as the code change"; "Default to
  delete"; link instead of copying; docs "alive but frequently trimmed, like
  a bonsai tree"; a README orients and points onward.
- https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions —
  Nygard's ADRs: one short record per decision, sequential ids never reused,
  a reversed decision kept and marked superseded.
- https://adr.github.io/madr/ — MADR 4.0.0 bare and minimal templates;
  `NNNN-title.md`; statuses proposed, accepted, rejected, deprecated,
  superseded.
- https://docs.arc42.org/home/ — twelve sections, "travel light"; the
  useful subset for a small project is goals, context, building blocks and
  decisions, which fit one architecture file plus the decision log.
- https://c4model.com/diagrams — context and container diagrams are enough
  for most teams (from a search summary; not verified on the page).
- https://github.com/RichardLitt/standard-readme/blob/main/spec.md — the
  README's required sections; long material "should be given in subsequent
  sections" or other files.
- https://keepachangelog.com/en/1.1.0/ — "Changelogs are for humans, not
  machines"; optional before there are outside users.
- https://go.dev/doc/comment — "Every package should have a package comment
  introducing the package"; doc comments do not explain internals.
- https://go.dev/blog/examples — `Example` functions with `// Output:` are
  verified by `go test`.

## Go layout

- https://go.dev/doc/modules/layout — the official guide: "it's recommended
  to keep the Go packages implementing the server's logic in the `internal`
  directory"; "it's a good idea to keep all Go commands together in a `cmd`
  directory"; reusable pieces are "split off to separate modules"; no
  `pkg/`.
- https://github.com/golang-standards/project-layout — "This is NOT an
  official standard defined by the core Go dev team"; calls itself overkill
  for small projects.
- https://github.com/golang-standards/project-layout/issues/117 — Russ Cox,
  2021: the name misrepresents the repo as official; most Go code does not
  use `pkg/`; the layout harms beginners. Locked 2023 with no rename.
- https://www.gobeyond.dev/standard-package-layout/ — Ben Johnson: main
  wires dependencies; the root package holds domain types.
- https://peter.bourgon.org/go-best-practices-2016/ — "no single best repo
  structure"; its `pkg/` advice is the part the community moved away from.
- Repository roots fetched: tailscale and prometheus use `cmd/`, `internal/`,
  `scripts/`, `docs/`, no `pkg/`; fzf and cobra are flat; `cli/cli` uses
  `pkg/` but is large. None follows golang-standards wholesale.

## Complaints the rules answer

- https://www.thoughtworks.com/radar/techniques/agent-instruction-bloat —
  long instruction files: important rules ignored, conflicts as it grows;
  hand-written beats generated.
- https://arxiv.org/abs/2602.11988 — generated codebase overviews in
  AGENTS.md lowered success and raised cost.
- Ousterhout, *A Philosophy of Software Design* (comments chapter, via the
  RAMCloud wiki) — "Increment i" above `i++`; comments repeating the
  identifier; comments describe what the code cannot, and are part of the
  interface.
- https://arxiv.org/pdf/2406.14836 — model-written comments that misdescribe
  intent or name code that does not exist.
- https://codex.danielvaughan.com/2026/06/07/agent-generated-documentation-quality-trust-codex-cli-verification-patterns/
  — hallucinated package names copied into real install instructions;
  invented rationale in decision records (vendor blog, secondhand).
- Practitioner threads (search snippets only): comments that narrate every
  line, banner every section and annotate every edit; agents deleting
  existing comments on a rewrite; facts copied across README, CLAUDE.md and
  AGENTS.md and drifting.
- Project AI policies found (Ghostty `AI_POLICY.md`, QEMU, NetBSD, curl)
  govern provenance and disclosure, not comment style; none is a comment
  standard.

## Not found

No authoritative writing on code citing documents by heading anchor with a
CI check on the anchors. That convention is this skill's own, carried over
from shhh and hearth, where it replaced about 2,600 story and section
references.
