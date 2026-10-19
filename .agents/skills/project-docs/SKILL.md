---
name: project-docs
description: The document set for a small or medium repository that agents edit and people read, and the process that keeps it true — README.md, AGENTS.md (and the CLAUDE.md shim), docs/ with its index, design documents, decisions, dated research and reference pages, .agents/skills, and code comments. Use when setting up or auditing a repository's documentation, when asked to "update the docs", "write AGENTS.md", "add a decision" or "add an ADR", when deciding where a fact belongs or whether something is a document, a skill, a comment or a check, when a change moves a documented fact (a command, a path, an invariant, a decision), when AGENTS.md or a document is growing, and when writing or reviewing a code comment. Also settles Go repository layout (cmd/ and internal/, no pkg/). Not for building or placing a skill (that is create-skill) or for prose style in user-facing copy.
metadata:
  author: rfizzle
  standard: agentskills.io
---

# Project documentation

A repository that agents edit has one documentation problem: nothing in it is
read end to end, so a wrong sentence survives until someone acts on it. The
design below is built around that. Every fact has one home, every home is
small enough to re-read, the mechanical drift is caught by a check, and a
change that moves a fact moves its sentence in the same commit.

Two rules carry most of it. **Each file answers one question.** **One home
per fact; everything else links.**

## The shape

| File | Reader | Question it answers | Size |
|---|---|---|---|
| `README.md` | a person deciding whether and how to use it | What is this, how do I install and run it? | one screen; links out |
| `AGENTS.md` | every agent, every session | Where is the code, how is it checked, what bites? | under 200 lines; aim for 80–150 |
| a client rules file (`CLAUDE.md`, `.cursor/rules`, ...) | one client | nothing of its own: a pointer to `AGENTS.md` | 2 lines, or absent |
| `docs/README.md` | anyone opening `docs/` | What answers what, and when does each change? | a table |
| `docs/product.md` | a person judging a feature request | What is it, for whom, what will it never do? | pages |
| `docs/architecture.md` | a person judging a structural change | What are the big shapes, and why those? | pages; one diagram at most |
| `docs/decisions.md` or `docs/decisions/` | a person about to re-litigate | What was decided, when, why; what is still open? | one record per decision |
| `docs/research/<date>-<topic>.md` | a person asking "what did we look at?" | A dated snapshot | never edited |
| `docs/reference/` | a person who needs the exact value | Facts code comments can't carry: flags, config keys, wire formats | generated or golden-tested |
| `.agents/skills/<name>/SKILL.md` | an agent doing a particular kind of task | How is this kind of change made here? | under 500 lines |
| `doc.go` and doc comments | a reader of the package | What does this package promise? | the API doc; nowhere else |
| a code comment | the next editor of the line | Why is this line like this? | as long as the reason |

The lifetimes are the point. A fact filed under the wrong question outlives
its usefulness there: a roadmap in a design document is stale in a sprint; a
behaviour rule in a changelog is invisible; a command list copied into three
files is wrong in two of them within a month.

Nothing is created empty. `docs/` starts as `README.md` plus whichever of the
files above has a real reader today, and grows by one file when a new
question is actually asked. Diátaxis is useful as a lens for classifying a
page by the need it serves; it is not a folder template, and its own guidance
is not to create empty structures.

## One home per fact

Before writing a sentence, decide which question it answers; that names the
file. If the sentence is already true somewhere, link there and stop. The
duplicate is the drift.

- A **command** lives in the Makefile and is named by `AGENTS.md`. `README.md`
  shows the two or three a user runs; nothing else repeats the table.
- A **path** lives in `AGENTS.md`'s map. Design documents name no file,
  package or symbol: a document that names a function drifts silently the
  moment the function is renamed. The map from intent to code is `AGENTS.md`;
  the map from code to intent is a citation in a comment.
- A **decision and its reason** live in the decision record. Other documents
  state the result and may link the record.
- A **rule the model must follow** (what a tool does, when to pass an
  argument) lives where the model reads it: the tool's schema or description,
  or the prompt. The *reason* for the rule lives in the design document that
  the code cites.
- A **procedure** (how to add a view, regenerate the proto, cut a release) is
  a skill, not a document and not a section of `AGENTS.md`.
- A **planning artefact** (story, epic, ticket, sprint, roadmap) lives in the
  tracker. Nothing in the repository names one.

## AGENTS.md

The one agent instruction file, tool-neutral, read by every agent in every
session. Its budget is attention, not disk: past about 200 lines adherence
drops and the important lines are lost among the rest. For each line ask
"would removing this cause a mistake?" If not, cut it.

What goes in, in this order:

1. **One paragraph**: what the project is and the one-sentence shape.
2. **Scope and restraint**: the smallest correct change; no drive-by
   refactors, renames, reformatting, dependency bumps or doc expansion.
3. **Documentation**: a pointer to this skill's rules as they apply here, in
   five or six bullets, and the change rule below.
4. **Commands**: a table of task to `make` target. Only targets that exist.
5. **Testing**: the tiers, the fixtures at the seams, the one or two rules
   that make tests uncacheable or flaky here.
6. **Version control**: commit scope, subject format, what never to run.
7. **Architecture**: the directory map with one phrase per entry, and the
   import direction. Mark what does not exist yet as "(planned)".
8. **Invariants**: the five to ten facts a reviewer checks every diff
   against.
9. **Gotchas**: the traps that cost someone an hour, each with its symptom.

What stays out: anything the agent does correctly without being told;
file-by-file descriptions; style rules a linter enforces ("never send an LLM
to do a linter's job"); procedures (a skill); design reasoning (`docs/`);
anything that is true this week only.

- **Every path and command in it must be checkable.** `make docs-check`
  verifies that a backticked path exists and a `make` target is in the
  Makefile. A planned path says "(planned)" on its line.
- **Nested `AGENTS.md`** only for a sub-tree with its own toolchain (an
  `App/` beside a Go core). The nearest file wins; keep the nested one to the
  delta.
- **A client's own rules file points at `AGENTS.md` and holds nothing
  else**, or does not exist. `CLAUDE.md` is the line `@AGENTS.md`; other
  clients' files are their equivalent include or a symlink. A client file
  with content of its own silently forks the rules, because most clients
  read only their own file when it exists. Never a copy.
- **Hand-written, never generated.** A draft a tool generates from the tree
  is a starting inventory to prune, not a file to commit. Rules that must
  hold every time are hooks or CI checks, not sentences; the file is
  advisory.

## README.md

For the person, not the agent. Title, one-sentence description, status if
pre-release, install, the shortest working use, where the documents are,
licence. Long material goes below the fold or in `docs/`. No agent rules, no
architecture, no command table beyond what a user runs. A library's README is
also its front page on pkg.go.dev, so the first paragraph must stand alone.

## docs/

### The index

`docs/README.md` is a table: file, the question it answers, when it changes.
It is the only place the document set is enumerated. A new file is added to
the table in the same commit that creates it.

### Design documents

`product.md` and `architecture.md`, plus a domain document (the things in the
system and how work flows between them) when the product has a vocabulary
the code must share. Written for a person judging a change, not for an agent
making one.

- **Headings are anchors; treat one like an exported symbol.** Code cites a
  heading as `// See docs/architecture.md#the-heading`. Renaming one breaks
  every citation; rename deliberately and fix the citations in the same
  commit.
- **A section nothing cites is either wrong or unnecessary.** The citations
  are the test that the document still describes the software. The check
  lists uncited headings; read that list at every audit.
- **No symbols, no roadmaps, no open questions.** Symbols drift; roadmaps and
  questions are planning, which lives in the tracker or the decision log.

### Decisions

A decision is recorded once, where it is made, with a permanent id, the date,
its status, the context, the decision and its consequences. A superseded
decision is never deleted; it gains a status naming its successor. Numbers
are never reused.

Two layouts, same record. A single `docs/decisions.md` (a table or one
heading per decision) while every decision fits on a screen. One file per
decision in `docs/decisions/NNNN-slug.md` (the MADR minimal template) once
reasons run to paragraphs or two people edit the log at once. The id, not
the file, is the identity, so moving from one layout to the other breaks
nothing.

Record a decision when a reviewer could reasonably ask "why not the obvious
alternative?" and the answer is not in the code. Do not record what a
convention already answers. The reason in a record is the reason a person
gave or the code evidences; an agent that infers one writes "reason not
recorded" and asks. An invented rationale is worse than none, because the
next reader will defend it.

### Research

Anything time-bound: a competitor survey, a platform's API table, a UX
sketch, a spike's result. The file name starts with the date. The file is
never edited afterwards and never cited from code; a new look gets a new
dated file. This is what keeps the design documents free of material that
rots.

### Reference

Exact facts a person needs to look up and that doc comments cannot carry:
CLI flags, configuration keys, a wire format, a file layout on disk. Each
page is generated from the code by a `make docs` target or is a golden file a
test regenerates with an update flag, so that it cannot disagree with the
binary. A hand-written reference page is a bug report waiting to happen.

The API itself is documented in Go doc comments: a package comment in
`doc.go` stating the package's contract, and a comment on every exported
identifier whose name does not say what it returns, costs or mutates.
Runnable `Example` functions with `// Output:` are documentation `go test`
checks. Nothing in `docs/` restates them.

## Skills

A skill is a procedure that matters only for some tasks: how to add a view,
how to change the proto, which test tier a change belongs in, how a release
is cut. It loads on demand, so it can be detailed where `AGENTS.md` cannot.
This section decides what is a skill and what makes one worth having; the
mechanics (location under `.agents/skills`, the `.claude/skills` symlink,
frontmatter, description wording, size limits) are the `create-skill`
skill and are not repeated here.

- **Something is a skill when it is a procedure a tool or a reviewer acts
  on.** A fact is a line in `AGENTS.md`; a reason is a section in `docs/`;
  a rule that must hold every time is a check. None of those is a skill.
- **An effective skill is a procedure with a definition of done**: the
  steps, the choice points with the rule for each, and the command that
  proves it was followed. Philosophy, history and generic advice are cut; an
  agent reading a skill is mid-task and wants the next step.
- **A skill that restates the language's conventions is noise.** The stdlib
  and `go.dev` are the reference; a skill earns its place with what this
  repository does differently and why.
- **`AGENTS.md` routes to skills** in a sentence or a short table, naming
  the paths or situations that trigger each. `docs/` never describes a
  procedure.

## Code comments

A comment is written for the next person to edit the line, in plain prose,
and it earns its place by saying something the code cannot. Readable code
needs no comment saying what it does; it may still need one saying why it is
there. Before writing one, decide which of these it is, and if it is none,
delete it:

- **Why this and not the obvious alternative.** The reader is about to
  "simplify" it back.
- **What breaks if it changes, and what that looks like.** Lead with the
  symptom, because the symptom is what the next person will see.
- **A constraint from outside this file.** A protocol quirk, an OS limit, a
  vendor's dialect, a measured number.
- **A non-obvious invariant**, especially one the types do not hold.
- **A deliberate no-op or ignored value.** One line separates a choice from
  an unfinished edit.

What never earns its place:

- **A story, ticket, epic, sprint or plan id.** It answers "when was this
  built", which the code should not ask, and it points at something the
  reader cannot open. Say what the code does and why.
- **History as explanation.** "X added the fifth option" describes a
  calendar. Keep history only when it stops a regression, and then name what
  broke, not when.
- **A restatement of the signature**, a pointer the sentence already
  contains, or a cross-reference in place of the reason.
- **Obscurity.** A comment is not a place to be clever, terse or allusive.
  If a reader needs the author beside them to parse it, it has failed.
- **A reference in data.** A doc or spec pointer in a string literal becomes
  test output, an error message or a committed fixture.
- **Edit annotations and banners.** "Added for X", "Updated to handle Y",
  "Fixed", a row of dashes, a section title in a comment, an emoji. The
  commit message carries the change; the code carries the structure.
- **A reason you were not given.** A comment that explains a choice the
  author never stated, or that names code that does not exist, is a
  plausible lie. If the reason is unknown, say what the code does and leave
  the why to the person who knows it.

Existing comments are the previous editor's reasons. A targeted edit keeps
them; a rewrite that drops one says so in the report, and a comment that has
become false is corrected, not deleted.

Go doc comments have a fixed form: a full sentence that begins with the name
("Package store ...", "Resolve returns ...", "Empty reports whether ..."),
saying what is returned, mutated or guaranteed (zero value, concurrency),
never how it is implemented. The package comment in `doc.go` is the
package's contract: what it promises and what it refuses to know.

The citation convention ties comments to design documents: the reason in
prose first, then the pointer, and only for a product or design decision:

```go
// Act-tier tools always go through the approval gate: the core cannot know
// whether a tool changed something outside the workspace, so it assumes it
// did. See docs/model.md#approval.
```

Local mechanics get the sentence and no pointer. Density is the test, not
length: the most valuable comments are often the longest, and the least
valuable are one-line cross-references. Worked before/after pairs are in
`references/comments.md`.

## The change rule

A change is not done until every document it falsifies is true again, in the
same commit. This is the whole maintenance process; everything else is a
check that it was followed.

Before finishing a change, ask in order:

1. Did it move a **command, path, target, flag or version**? Fix the line in
   `AGENTS.md` or `README.md`. These are objective facts; fix them without
   asking.
2. Did it change a **behaviour a design document describes**? Update the
   section, and check whether the citing comment's reason still holds.
3. Did it make or reverse a **decision**? Add or supersede the record. If
   the decision was not yours to make, say so in the report instead of
   recording it.
4. Did it add a **trap** that cost real time? One bullet under Gotchas,
   symptom first.
5. Did it change what a **tool's schema, description or prompt** says? Name
   which, and cite the section that holds the reason.
6. Is every command, flag or path you wrote one you ran or opened in this
   session? A documented command is a command that was run; an invented
   one lands in someone's install instructions.
7. Run `make docs-check`.

An agent fixes objective facts on its own. A change to intent, policy or a
decision is proposed in the report and left to a person. A doc-only commit is
`docs(scope): ...`; a doc change that rides with code is in that code's
commit, because that is the commit that made it necessary.

Delete rather than archive. A document nobody reads is a document that lies
to the one person who does; git history keeps the old text.

## make docs-check

The mechanical half. One stdlib-only Go program under `scripts/docscheck`,
run by `make docs-check`, which `make ci` includes. The program is this
skill's `scripts/docscheck/`; vendoring the skill vendors it too, hashed in
the lock file like the skill, so it is never edited in a project. Per-repo
differences are flags on the `make` target (`-workitem`, `-agents-max`,
`-skip`). It fails on:

- a Markdown link whose file or heading does not exist;
- a `// See docs/<file>.md#<heading>` citation in Go that does not resolve;
- a citation of `docs/research/`;
- a backticked path in `AGENTS.md` or `README.md` that does not exist and is
  not marked "(planned)";
- a `make <target>` in any document that the Makefile lacks;
- a required-Go-version claim in a root file that disagrees with `go.mod`;
- a work-item id in a document or in code;
- a research file whose name does not start with a date;
- an `AGENTS.md` over the line limit.

And it lists, without failing, the design headings nothing cites.

What not to add: a prose linter (Vale), a generated table of contents, or an
LLM pass over the docs. They cost more than they catch at this scale. Add a
link checker for external URLs only on a schedule, never in the commit gate.

## The audit

At each milestone, or when a session reports a stale instruction, one pass:

1. `make docs-check`; fix everything it names.
2. Read `AGENTS.md` top to bottom against the repository. Delete every line
   the agent would get right anyway. Resolve any two lines that disagree:
   an agent facing a contradiction picks one at random.
3. Read the uncited-headings list. Each is a section to cite, rewrite or
   delete.
4. Confirm every row of `docs/README.md` still names a file that answers its
   question, and no file is missing from the table.
5. Prune, then add. The file set should be alive and trimmed, never
   growing by accretion.

## Adopting a repository

For a repository with no `docs/`, or one with too much:

1. Inventory every Markdown file. For each, write the one question it
   answers. A file with two answers is split; a file with none is deleted;
   two files with the same answer are merged.
2. Write `AGENTS.md` fresh in the order above, from the Makefile and the
   tree, not from the old file. Then diff against the old one and carry over
   only the gotchas and invariants that earn a line.
3. Reduce `README.md` to the person's questions; move the rest.
4. Create `docs/README.md` with the table, then only the design files that
   have a reader today. Date and freeze anything time-bound under
   `docs/research/`.
5. Record the decisions that are already made and still contested.
6. Copy `scripts/docscheck`, add the `docs-check` target to `ci`, run it,
   and fix what it finds before the first commit of the new shape.
7. Move procedures out of documents into skills (`create-skill` has the
   mechanics).

## Go repository layout

Follow the official module layout guide (`go.dev/doc/modules/layout`), not
`golang-standards/project-layout`. The latter is not a standard: its own
README says so, the Go team objected to its name and to `pkg/`, and no
major Go repository follows it wholesale.

- A command: `cmd/<name>/main.go`, thin, assembly only. Main wires
  dependencies and parses flags; nothing else does either.
- Everything the command needs: `internal/`, dependencies pointing down. Keep
  packages internal as long as possible. A piece another module wants is
  split into its own module, not moved to `pkg/`.
- A library: packages at the root, flat, no `cmd/` or `internal/` until a
  command or a private helper exists.
- `docs/`, `scripts/`, and non-Go siblings (`App/`, `web/`, `proto/`) sit at
  the root. No empty placeholder directories; a directory is created when it
  has a file.
- Each package has a `doc.go` whose package comment states what the package
  promises and what it refuses to know.

## References

- `references/templates.md`: skeletons for `AGENTS.md`, `README.md`,
  `docs/README.md`, a decision record, and the Makefile target.
- `references/comments.md`: before/after pairs for code comments.
- `references/sources.md`: what was read, with URLs and the line each
  source contributes.
- `scripts/docscheck/main.go`: the reference checker.
