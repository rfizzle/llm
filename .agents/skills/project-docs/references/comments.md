# Code comments: before and after

Each pair is a real change from a sibling repository. They are examples of
judgement, not a style to copy; the test is always the one in `SKILL.md`:
does this say something the code cannot?

## What the code already says

```go
- // increment the counter
- counter++
+ counter++
```

```go
- // DeleteSnippet deletes a snippet.
- func DeleteSnippet(id string) error
+ func DeleteSnippet(id string) error
```

A restatement is a second place to update and a second place to be wrong.
An exported name that says what it does needs no line; `Resolve()` and
`Append()` do, because the name does not say what they return, cost or
mutate.

## The story id

```go
- // S-114 added the fifth: the commands the generator did not pick.
+ // The fifth is the commands the generator did not pick.

- // Package scope holds a session's working scope: the directories the work
- // is allowed to reach (S-141).
+ // Package scope holds a session's working scope: the directories the work
+ // is allowed to reach. See docs/model.md#working-scope.
```

The id answers "when was this built", which is not a question the code
should ask, and it points at a tracker the reader cannot open. The prose was
already right; only the pointer was wrong, so rewrite the pointer and leave
the explanation alone.

## History standing in for a reason

```go
- // glamour filled rather than against the pane. Before S-147 it measured
+ // glamour filled rather than against the pane. It used to measure
```

The *when* was never the useful part. Where history prevents a regression,
keep it and say what broke:

```go
// The leading half was a strings.TrimSpace once, and it worked by accident.
```

## The pointer the sentence already contains

```go
- // Detail bodies indent, they do not re-grid (§6a).
+ // Detail bodies indent, they do not re-grid.
```

Cover the parenthetical. If the sentence still says the same thing, it was
noise.

## Edit annotations and banners

```go
- // ===================== Helpers =====================
- // Added retry here to fix the timeout issue (updated 2026-03)
  func withRetry(fn func() error) error {
+ // Three attempts with backoff: the provider's gateway drops the first
+ // connection after an idle hour, and the second attempt is what succeeds.
+ func withRetry(fn func() error) error {
```

The banner is structure the file already has; the annotation is the commit
message in the wrong place. What survives is the reason the retry exists.

## A reason nobody gave

```go
- // We use a mutex here for performance reasons.
+ // Guards cache; read from the request path and written by the refresher.
```

The first was invented: nobody said performance, and a mutex is not a
performance choice. If the reason is unknown, state what the lock protects
and leave the why to the person who knows it.

## Obscure

```go
- // see above; same trick as the other one
- n := len(buf) - 1
+ // The trailing byte is the terminator the parser added; the caller never
+ // sees it.
+ n := len(buf) - 1
```

A comment that needs its author beside it to parse has failed. Say the thing.

## A reference in data

```go
- {Label: "output · a program's own colours, re-painted (§10i)", ...}
+ {Label: "output · a program's own colours, re-painted", ...}
```

A string literal becomes test output or a committed fixture, and then
editing a heading means regenerating fixtures. Test names have the same
problem: `name: "approval card (§2)"` already said what it covered.

## Why, when the code is perfectly readable

```go
conn.SetMaxOpenConns(1)
```

Readable, and still wrong without a sentence: the workload is one interactive
process, WAL and a busy timeout handle concurrency, and a second connection
buys nothing while adding lock contention that is miserable to reproduce. No
citation; this is a local mechanic, not a product decision.

## A comment that earns every line

```go
// Gemini pairs tool results by function name, not by id. FunctionResponse.Name
// must be the name of the function called, and the API sends no
// functionCall.id at all; the ids in provider.ToolCall are ours. Don't
// "simplify" this back to putting ToolCallID in that field: it addresses
// every result to a function the model never called, and the model just
// calls again.
```

Long, and worth it. It names the alternative the reader is about to reach
for and leads with the symptom, *the model just calls again*, which is what
they will actually observe. The cause is invisible from the symptom; that is
exactly when a comment is worth writing.

## The citation, done right

```go
// Act-tier tools always go through the approval gate: the core cannot know
// whether a tool changed something outside the workspace, so it assumes it
// did. See docs/model.md#approval.
```

The reason is in the comment as prose; the pointer is to the long form, for
a reader who wants the trade-offs. A reader who never opens the document
still knows why the line is the way it is.

## A branch that is empty on purpose

```go
case hunk != nil:
    // "\ No newline at end of file" and blank separators.
case strings.HasPrefix(line, "+++ "):
```

One line is the difference between a deliberate no-op and an unfinished
edit. Nothing about the empty branch says which it is.

## A number that was chosen

```go
- const idle = 45 * time.Second
+ // 45s: measured against the slowest provider's keep-alive gap (30s) with
+ // headroom; below 35s the stream is cut mid-token on a cold model.
+ const idle = 45 * time.Second
```

A chosen number without its measurement is a number the next person will
"round".
