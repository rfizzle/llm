# llm · decisions

What was decided, when, and why. Ids are permanent and never reused; a
superseded decision keeps its row and changes only its status. Where nobody
wrote the reason down, the row says "reason not recorded" rather than guess
one.

The dates are the dates the behaviour landed in this repository, recorded on
2026-10-19.

| ID | Question | Decision | Why | Date | Status |
|---|---|---|---|---|---|
| D1 | Does an application's policy (meter, budget, tool policy, secret scrubbing, logging) live in the library? | No. The library holds the provider plumbing; policy stays in the application, and nothing in the library names one. | The module is shared by more than one application; any further reason was not recorded. | 2026-10-18 | accepted |
| D2 | How does an application get a backend? | Each backend is its own package and registers its provider names when imported, the way a `database/sql` driver does; the root package imports none. | An application imports only the backends it uses and links only their vendor SDKs, so the set of built-in providers is the set it imported. | 2026-10-19 | accepted |
| D3 | Do the vocabularies (failure classes, flows, capabilities, effort, attachments, cache, catalog, decisions, tool values) live in the root package? | No. Each is a package of its own; the root keeps the message, tool-call, stream and usage types, the registry and the caller's identity. | The root held every vocabulary at 1,633 lines of code and 201 exported names. | 2026-10-19 | accepted |
| D4 | Does the price table need the network? | No. One public table of price, context window and thinking level, with a snapshot built in as the floor under the download. | One download serves the spend meter, the context gauge and the reasoning ladder; the snapshot inside the binary is the floor under that download. | 2026-10-18 | accepted |
| D5 | Where does the ChatGPT plan sign-in live, and how does it work? | In the library: OpenAI's OpenID Connect sign-in with PKCE on a loopback callback, refresh under a cross-process lock, revocation. | So a session can spend the plan instead of API credit, and PKCE on a loopback callback is OpenAI's contract. Why the library holds it rather than each application: reason not recorded. | 2026-10-19 | accepted |
| D6 | Who decides where the sign-in record is stored? | The application, through a store it may implement; the directory-backed store is one implementation. | An application that keeps the credential in a keychain behind its own process cannot use a directory. | 2026-10-19 | accepted |
