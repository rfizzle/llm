package llm

import "context"

// TokenSource is a bearer that moves under a session. refused is the bearer
// the last request was refused with, or empty. A source still holding refused
// refreshes before it answers; one that answers refused again could not. An
// error is classified unauthorized unless it is a network failure.
//
// It is the one handle the ChatGPT plan route holds on its credential: an
// access token comes out of it and nothing else does, so a refresh token is
// never in this package, on a request, or in an error built here.
type TokenSource interface {
	Token(ctx context.Context, refused string) (string, error)
}

// signedOut is the error behind ErrChatGPTSignedOut. It is a comparable value
// so that errors.Is matches it, and its words are read when it is printed,
// not when the program starts, so they name the caller set by SetApp.
type signedOut struct{}

func (signedOut) Error() string {
	if hint := caller().SignInHint; hint != "" {
		return "not signed in to ChatGPT; run `" + hint + "`"
	}
	return "not signed in to ChatGPT"
}

// ErrChatGPTSignedOut is the resolution failure of a plan session with no
// credential to send. It names the command, because the setup that fixes it
// is a sign-in in a browser, not a key anybody can paste.
var ErrChatGPTSignedOut error = signedOut{}
