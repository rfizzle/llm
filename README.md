# llm

A Go library for talking to large language models, shared by more than one
application. It holds the provider plumbing (message, tool-call, stream and
usage types, one package per backend, pricing and the ChatGPT plan sign-in)
and none of any app's policy: no meter or budget, no tool policy, no secret
scrubbing, no log file.

Status: pre-1.0, versioned by tags v0.N.0.

## Install

```
go get github.com/rfizzle/llm
```

## Use

```go
import (
	"github.com/rfizzle/llm"
	_ "github.com/rfizzle/llm/anthropic" // registers "anthropic"
)

llm.SetApp(llm.App{Name: "acme", EnvPrefix: "ACME"})

// Reads ACME_API_KEY, then the vendor's own variable.
p, err := llm.Resolve("anthropic", llm.ResolveOpts{Model: "claude-demo"})
if err != nil {
	return err
}
events, err := p.StreamCompletion(ctx,
	[]llm.Message{{Role: llm.RoleUser, Content: "Say hello."}},
	llm.CompletionOpts{Model: "claude-demo"})
if err != nil {
	return err
}
reply, err := llm.Collect(ctx, events)
// reply.Text, reply.Calls, reply.Usage, reply.Stop
```

A backend is linked only when imported, the way a `database/sql` driver is;
`llm.Resolve` answers for the imported ones. `llm.SetApp` names the caller
once: the variables a key is read from, the prefix of cache keys, the
sign-in hint a person is shown. Runnable examples, including a tool call,
pricing a usage and the ChatGPT sign-in, are in `example_test.go`,
`pricing/example_test.go` and `chatgpt/example_test.go`.

The design is in `docs/`; start at `docs/README.md`. Licensed under the MIT
License.
