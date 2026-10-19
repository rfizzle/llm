# llm · architecture

`llm` is a Go library for talking to large language models, shared by more
than one application. This document is for a person judging a structural
change: what the module holds, what it refuses to hold, and the shapes it is
built from. Where each shape lives in the tree is in `AGENTS.md`; why each
was chosen, where a reason was given, is in [decisions.md](decisions.md).

## What it holds

What any application that talks to a model needs, and nothing that differs
between applications:

- the message, tool-call, stream and usage types every backend translates to
  and from;
- the backends, one package each: Anthropic's Messages API, OpenAI's chat
  completions, OpenAI's Responses API with the ChatGPT plan route, Google
  Gemini, OpenRouter, and any OpenAI-compatible endpoint;
- the vocabularies an application reads, one package each: the failure
  classes, the bounded calls a request can be, what a model can be asked,
  the reasoning level, attachments, prompt-cache lifetimes and marking, the
  model catalog, the Decisions API and the structured part of a tool result;
- failure classification with the provider's requested wait, and the stream
  idle deadline and keep-alive;
- the model price and context table, with a snapshot built in;
- the ChatGPT plan sign-in: OAuth with PKCE, refresh, revocation and a
  file-backed credential store.

## What it refuses to hold

An application's policy stays in the application:

- **The meter and any budget.** The library prices a request's usage; the
  application decides what to add up and when to stop.
- **Tool policy.** Which tools exist, which calls are allowed, and what a
  tool does.
- **Secret scrubbing and the diagnostic log.** The application hides its
  secrets from the model; the sign-in hands it the tokens it learns so that
  it can. The library writes no log file of its own.
- **Where the sign-in record lives**, and every command and screen a person
  reads.
- **The retry schedule.** The library classifies a failure, says whether it
  is one a session can come back from, and reads the wait the provider asked
  for; the loop that waits and asks again is the application's.

Nothing in the library names an application. The caller names itself once at
startup: the environment variables a key and a base URL are read from (before
the vendor's own), the prefix of every cache and routing key, the title sent
to OpenRouter, the group the plan route's tools travel in, and the sentence
that tells a person how to sign in. Until it does, the library says "llm".

## Backends resolved by name

Each backend registers its provider names, defaults, model catalog and the
reader for its SDK's error shape when its package is imported, the way a
`database/sql` driver does. Resolving a provider by name answers for the
imported backends and no others, and the root package imports none of them.

Why: an application links the vendor SDKs it ships and no others, and the
set of built-in providers is exactly the set it imported. An application that
holds a configured client of its own builds the provider directly instead.

## The vocabularies are packages

The root package holds only the message, tool-call, stream and usage types,
the provider registry and the caller's identity. Every vocabulary an
application or a backend reads is a package beside it.

Why: when the root held every vocabulary it carried 1,633 lines of code and
201 exported names. A vocabulary the root itself needs (an
attachment, a cache lifetime, a capability, the reasoning level, a flow) sits
below the root and never imports it.

## Failures, retry and keep-alive

Every backend's errors are classified into one closed set of failure classes,
matched by identity, never by their text. A class says whether the failure
is recoverable (a rate limit, an overloaded server, a network failure) and
carries the wait the provider named, if it named one.

Every stream runs under an idle deadline that each event pushes forward: a
token, a thinking delta, an argument fragment, a keep-alive comment. A
request that is accepted and then never written to again is otherwise
indistinguishable from a model thinking hard, and an unattended caller has
nobody to press a key. An expired deadline cancels the request and ends it
as a network failure, which is a class the caller's retry already waits out.

A gateway holds a quiet stream open with an SSE comment or a ping. Each
dialect hands it to the caller as an event that carries nothing, so a
surface can tell a quiet live stream from one that has said nothing; where an
SDK swallows the ping, it is read off the response body on its way in.

## Pricing is a snapshot table

One public table carries what a model costs, how much context it has and how
it spells its thinking level, so one download serves the spend meter, the
context gauge and the reasoning ladder. A snapshot of it is built into the
library as the floor under that download, and is regenerated from the public
table with `make model-data`.

The table prices; it does not budget. It answers a cost and whether it found
the model, and what an unpriced request means is the caller's call. A
request's prompt count includes what the provider served from its prompt
cache and what it wrote into it; the three are priced apart, because a cache
read costs a fraction of the input rate and a cache write a premium over it.

## The ChatGPT plan sign-in

A person can sign in with their ChatGPT plan so that a session spends the
plan instead of API credit. The contract is OpenAI's: an OpenID Connect
sign-in with PKCE, answered on a loopback callback at 127.0.0.1; an access
token that lasts an hour and a refresh token that rotates on every refresh;
and a revocation endpoint for signing out. Everything that can go wrong in
that exchange (a forged callback, a token minted for another client, a
different account answering, two processes spending one refresh token) is
checked before anything is written.

The record is a credential. The application chooses where it lives: the
directory-backed store is a directory in the application's data directory,
created owner-only and never the configuration directory a person commits
with their dotfiles; an application that keeps it in a keychain supplies its
own store. The tokens leave the sign-in only as the bearer a request is sent
with and through a callback that lets the application hide them from the
model; nothing logs a URL, a body or a token.

Two accounts on one record, in one process or two, refresh one at a time.
A refresh holds the account's lock and then the record's lock and reads the
record again once both are held, so the process that waited finds the token
the first one fetched instead of spending the refresh token a second time.
A token is never refreshed before the server's earliest refresh time unless
it was refused, because refreshing early rotates every other process's copy
out from under it. A caller that waits longer than thirty seconds for another
process's refresh gets a deadline error, which is a passing failure and not a
signed-out account.
