# llm

A Go library for talking to large language models, shared by more than one
application. It holds the provider plumbing and none of any app's policy.

```
go get github.com/rfizzle/llm
```

## What it holds, and what it doesn't

It holds what any LLM app needs:

- the message, tool-call, stream and usage types (`llm.Message`, `llm.Tool`,
  `llm.ToolCall`, `llm.StreamEvent`, `llm.Usage`);
- the backends: Anthropic, OpenAI chat, OpenAI Responses and the ChatGPT plan
  route, Gemini, OpenRouter and any OpenAI-compatible endpoint, resolved by
  name with `llm.Resolve`;
- failure classification and retry, stream idle and keep-alive, prompt-cache
  marking, reasoning effort and model listing;
- `llm/pricing`, the model price and context table with a snapshot built in;
- `llm/chatgpt`, the ChatGPT plan sign-in: OAuth with PKCE, refresh, revoke
  and a file-backed credential store.

It does not hold the app's policy. These stay in the app:

- the meter and any budget: the library prices a `Usage`, the app decides
  what to add up and when to stop;
- tool policy: which tools exist, which calls are allowed, and what a tool
  does;
- secret scrubbing and the diagnostic log: the app hides its secrets from the
  model (`chatgpt.Options.OnTokens` hands it the tokens the sign-in learns),
  and `llm` writes no log file of its own;
- where the sign-in record lives, and every command and screen a person reads.

Nothing in the library names an app. It says "llm" until the app says
otherwise.

## Name the app

Call `llm.SetApp` once at startup. Everything a backend writes that is the
caller's rather than a vendor's comes from it: the environment variables a key
and a base URL are read from (`ACME_API_KEY` and `ACME_BASE_URL`, before the
vendor's own), the prefix of every cache and routing key, the `X-Title` sent
to OpenRouter, the group the plan route's tools travel in, and the command a
person is told to run to sign in.

`chatgpt.Options.App` follows what was given to `llm.SetApp` when it is left
zero, so an app names itself once. Set it only for an account that must say
something different.

Resolve a provider from the environment:

```go
llm.SetApp(llm.App{
	Name:       "acme",
	EnvPrefix:  "ACME",
	SignInHint: "acme login chatgpt",
	URL:        "https://example.com/acme",
})
// With ACME_API_KEY set:
p, err := llm.Resolve("openai-responses", llm.ResolveOpts{Model: "gpt-demo"})
```

This is `Example_setApp` in `example_test.go`.

## Stream a turn with a tool call

`StreamCompletion` returns a channel of events. `llm.Collect` reads it to the
end; an app that draws as the answer arrives reads the channel itself.

```go
events, err := p.StreamCompletion(ctx,
	[]llm.Message{{Role: llm.RoleUser, Content: "What does go.mod say?"}},
	llm.CompletionOpts{
		Model: "demo-model",
		Tools: []llm.Tool{{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  []byte(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		}},
	})
if err != nil {
	return err
}
reply, err := llm.Collect(ctx, events)
// reply.Text, reply.Calls (the finished tool calls), reply.Usage, reply.Stop
```

To answer a call, append the assistant message with its `ToolCalls`, then a
`RoleTool` message with the result and the call's `ToolCallID`, and stream
again. `Example_streamAToolCall` runs this against an in-process server
(`internal/testhttp`), so it needs no network; an app passes its own
`http.Client` the same way through `llm.NewOpenAIResponsesWith`.

## Price a Usage

`pricing.Table.Price` takes the counts a `Usage` carries and returns the cost
in dollars. The prompt count includes the cached tokens, as `Usage` reports it.

```go
table := pricing.Snapshot() // or pricing.Load(), which lays a downloaded table over it
u := reply.Usage
cost, found := table.Price("demo-model",
	int64(u.PromptTokens), int64(u.CompletionTokens),
	int64(u.CachedTokens), int64(u.CacheCreationTokens))
```

`found` is false for a model the table has no price for; the app decides what
an unpriced request means. `pricing.SnapshotJSON` returns the built-in table
as the JSON document it is stored in, for an app that seeds a cache with it.
The runnable forms are the end of `Example_streamAToolCall` and
`pricing/example_test.go`.

## Sign in with a ChatGPT plan

`chatgpt.FileStore` is a directory the app supplies, in its own data directory:
the record is a credential, and the directory is created owner-only.

```go
acct, err := chatgpt.Open(chatgpt.Options{Store: chatgpt.FileStore{Dir: dir}})
if err != nil {
	return err
}
if _, err := acct.Login(ctx, chatgpt.LoginOptions{Say: os.Stdout}); err != nil {
	return err
}
p, err := llm.Resolve("chatgpt", llm.ResolveOpts{TokenSource: acct})
```

`Login` opens a browser and waits on a loopback callback, so `Example_signIn`
in `chatgpt/example_test.go` is compiled and not run. `Example_signedOut` runs
the other half: an account with no record answers `llm.ErrChatGPTSignedOut`,
whose text names the app's `SignInHint`. Match these errors with `errors.Is`,
never by their text.

Two accounts on one directory, in one process or two, refresh one at a time.
A caller that waits longer than thirty seconds for another process's refresh
gets a deadline error, which is a passing failure and not a signed-out account.

## Versions

`llm` is versioned by tags, `v0.N.0`. A change to the library is committed and
tagged in this repository, and an app requires the tag it was built against.
A tag, once an app has recorded it, is never moved: a fix is a new patch
version, because module caches and `go.sum` already hold the old content.
Publishing a tag is the maintainer's step, and it comes before any app commit
that requires it is pushed.

Licensed under the MIT License.
