---
name: release
description: How a version of github.com/rfizzle/llm is cut. Use when cutting a tag or release of this module, when choosing the next version number, or when an app needs a library change it can require.
---

# Cutting a release

`llm` is versioned by tags, `v0.N.0`. An app requires the tag it was built
against, so a tag is a promise about content that module caches and `go.sum`
files already hold.

## Steps

1. **The change is committed here first**, with `make ci` green, and tagged
   here; an app then requires the tag.
2. **Choose the number.** The latest tag is
   `git tag --list 'v*' --sort=-v:refname | head -1`. A new feature or a
   breaking change is the next minor (`v0.N+1.0`); a fix is the next patch
   (`v0.N.1`).
3. **Never move a tag** an app has recorded, even to fix a mistake in it: a
   fix is a new patch version, because module caches and `go.sum` already
   hold the old content.
4. **Tag annotated**, on the commit `make ci` passed on:
   `git tag -a v0.N.0 -m "v0.N.0: <what an app gains>"`.
5. **Publishing the tag is the maintainer's step.** An agent stops after the
   commit (or after the local tag, if asked for it) and never pushes. The tag
   is pushed before any app commit that requires it is pushed.
6. **The app requires the tag**, not a pseudo-version:
   `go get github.com/rfizzle/llm@v0.N.0` in the app.

## Done when

- `git describe --exact-match HEAD` names the new tag, or the report says
  the tag was left to the maintainer.
- `git cat-file -t v0.N.0` prints `tag` (annotated, like every tag before
  it).
- In the app, `go list -m github.com/rfizzle/llm` prints the new tag.
