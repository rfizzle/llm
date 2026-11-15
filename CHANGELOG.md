# Changelog

All notable changes to this module are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the module is
versioned by tags.

## [0.7.1] - 2026-11-15

### Fixed

- `ResolveOpts.HTTPClient` is honoured by every provider constructor, not only
  the ChatGPT plan route. The OpenRouter, OpenAI, OpenAI-compatible, Anthropic,
  Gemini and Responses providers built their own client and ignored it. A given
  client now carries every request (OpenRouter and OpenAI layer their header
  and cache transports over its transport, keeping its timeout and redirect
  policy). With no client given, behaviour is unchanged.
