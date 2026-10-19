# AGENTS.md

`llm` (`github.com/rfizzle/llm`) is a Go library for talking to large language
models, shared by more than one application. It is the provider plumbing: the
message, tool-call, stream and usage types, one package per backend, the
vocabularies an app reads, pricing and the ChatGPT plan sign-in. It holds no
application's policy and names no app. The design is in `docs/` (planned).

## Scope and restraint

Make the smallest change that is correct, safe and verifiable, following the
local pattern. Don't refactor, rename, reformat, update dependencies, rewrite
tests or expand documentation unless the requested behaviour needs it. Every
exported name is API another module builds against: renaming or removing one
is a breaking change, never a drive-by.

## Documentation

`project-docs` is the standard (the document set, comments, the same-commit
rule); `sound-patterns` settles style (default to what the Go stdlib does).

- Each document answers one question; `docs/README.md` is the table.
- Documents name no code symbol; code cites documents:
  `// See docs/<file>.md#<heading>`. Headings are anchors; renaming one
  means fixing its citations in the same commit.
- A change that moves a command, path, behaviour or decision updates the
  document in the same commit. `make docs-check` verifies the mechanical
  part.
- Never a story, ticket or plan id in a document, comment or test name.
- `docs/research/` (planned) is dated, never edited, never cited.

## Commands

| Task | Command |
|------|---------|
| Format | `make fmt` |
| Static checks | `make fmt-check vet lint` |
| Hermetic tests | `make test` |
| Test one package | `go test ./<pkg>` |
| Integration tests (network or credentials) | `make integration` |
| Docs links, paths, targets and ids | `make docs-check` |
| Shared skills match their source | `make skills-check` |
| Refresh shared skills from `../skills` | `make skills-sync` |
| Regenerate the built-in model-data snapshot | `make model-data` |
| Everything CI runs | `make ci` |
| List every target | `make help` |

## Testing

- `make test` is hermetic: no listener, clipboard, daemon or network. Tests
  that cross a real boundary carry a build tag: `integration` (run by
  `make integration`) and `contract` (the real loopback listener in
  `chatgpt/login_contract_test.go`; run it with
  `go test -tags=contract ./chatgpt`). `make vet` compiles both tags, so a
  tagged file that stops compiling fails the gate.
- No TCP listener in the hermetic tier. Providers are tested against
  in-memory origins from `internal/testhttp`, an `http.RoundTripper` that
  dispatches to handlers, so a test keeps HTTP semantics without a socket.
- `internal/llmtest` is what the backends' tests share: the `acme` app their
  pinned strings were captured under (each backend's `main_test.go` sets it),
  the fake endpoints, the stream drainers and the fixtures every dialect is
  fed. It imports the root package, so the root package's own tests cannot
  import it and keep their own copies.
- The sign-in is tested against an in-memory authorization server
  (`chatgpt/fake_test.go`); no test reaches OpenAI.
- `make test` is cacheable on purpose: never add `-count=1`, and never
  `os.Chdir` or `t.Chdir` in a test.

## Version control

- Never `git add -A`, `git add .` or `git commit -a`. Name the paths.
- Commit on the checked-out branch, `master` included.
- Subject: `type(scope): subject`, imperative, lower case, no trailing period,
  under 72 characters; `!` after the type marks a breaking change. A subject
  that needs "and" is two commits.
- Tags and pushes are the maintainer's. Never push, never tag, never move a
  tag.

## Architecture

Package doc comments (`doc.go`) are the detailed map.

```
.                     the root package llm: message, tool-call, stream and
                      usage types, the provider registry, the caller's identity
anthropic/            Anthropic Messages API backend
openai/               OpenAI chat completions, and what every chat-completions
                      dialect shares (converters, stream reader, model list)
openai/responses/     Responses API backend and the ChatGPT plan route
gemini/               Google Gemini backend
openrouter/           OpenRouter backend, through openai/
compat/               any OpenAI-compatible endpoint, through openai/
failure/              the closed failure taxonomy and retry hints
flow/                 the bounded calls a request can be
capability/           what a model can be asked; profile declarations
effort/               the reasoning level, before each dialect spells it
attachment/           pictures, files and recordings on a message
cache/                prompt-cache lifetimes, marking, reported fields
catalog/              which models each provider serves; context floor
decision/             the Decisions API vocabulary
toolvalue/            the structured part of a tool result
pricing/              model price and context table, snapshot in models.json
chatgpt/              ChatGPT plan sign-in: OAuth with PKCE, refresh, revoke
chatgpt/credstore/    where the sign-in record lives: Store and FileStore
internal/cachemarks/  where prompt-cache markers go, chosen once for both paths
internal/idle/        the idle deadline every stream runs under
internal/llmtest/     test support shared by the backends
internal/testhttp/    in-memory HTTP origins for tests
scripts/              docscheck and skillscheck (vendored), model-data.py
```

Dependencies: backends import the root and the vocabularies; the root
imports `attachment`, `cache`, `capability`, `effort` and `flow`, so those
never import the root. Nothing imports a backend except that `compat`,
`openrouter` and `openai/responses` build on `openai`.

## Invariants

- **No app policy.** The meter and any budget, tool policy, secret scrubbing,
  the diagnostic log, where the sign-in record lives and every command or
  screen a person reads stay in the app.
- **Nothing names an app.** Everything that is the caller's (env prefix, cache
  and routing key prefix, sign-in hint) comes from `llm.SetApp`; the default
  is "llm" (env prefix `LLM`).
- **A backend is linked only when imported.** Each registers its names from
  its `init`; `llm.Resolve` answers for imported backends and no others, and
  the root package imports none.
- **Pricing is a table, not a budget.** It prices a usage and reports whether
  it found the model; what an unpriced request means is the app's call.
- **Failures are classes, matched with `errors.Is`**, never by their text.
- **The sign-in logs no URL, body or token.** Tokens leave `chatgpt` only as
  the request's bearer and through the `OnTokens` callback.
- **Cache-marker positions are chosen once**, in `internal/cachemarks`, for
  both the Anthropic backend and the gateway transport.
- **`scripts/docscheck`, `scripts/skillscheck` and the shared skills are
  vendored.** Edit them in `../skills`, then `make skills-sync`.

## Gotchas

- **Another process's ChatGPT sign-in fails after this one refreshes.** The
  refresh token rotates on every refresh, so refreshing before the server's
  earliest refresh time (unless the bearer was refused) spends it and rotates
  every other process's copy out from under it. The refresh holds the account
  lock, then the file lock, then rereads the record; keep that order
  (`chatgpt/token.go`).
- **A redirect from the token endpoint fails instead of being followed.** A
  307 or 308 would re-post the form, refresh token included, to wherever
  `Location` names, so the account copies the caller's `http.Client` and
  refuses redirects (`chatgpt/account.go`). Don't make it follow them.
