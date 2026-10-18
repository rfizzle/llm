package chatgpt_test

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/rfizzle/llm"
	"github.com/rfizzle/llm/chatgpt"
)

// Example_signedOut opens an account over a FileStore and asks it for a
// token before anyone has signed in. The app names itself once, with
// llm.SetApp; Options.App is left zero and follows it.
func Example_signedOut() {
	llm.SetApp(llm.App{Name: "acme", EnvPrefix: "ACME", SignInHint: "acme login chatgpt"})
	dir, err := os.MkdirTemp("", "chatgpt-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()

	acct, err := chatgpt.Open(chatgpt.Options{Store: chatgpt.FileStore{Dir: dir}})
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = acct.Token(context.Background(), "")
	fmt.Println(err)
	// Output: not signed in to ChatGPT; run `acme login chatgpt`
}

// Example_signIn is the sign-in a login command runs, and the provider that
// spends the plan afterwards. The authorization server is a fake held in
// memory (example_fake_test.go) and the browser is a function that calls the
// loopback callback back, so it runs without a window or the network; an app
// leaves Client and Browser zero and passes its own data directory.
func Example_signIn() {
	llm.SetApp(llm.App{Name: "acme", EnvPrefix: "ACME", SignInHint: "acme login chatgpt"})
	dir, err := os.MkdirTemp("", "chatgpt-example-")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	server, client := newExampleAuth()

	acct, err := chatgpt.Open(chatgpt.Options{Store: chatgpt.FileStore{Dir: dir}, Client: client})
	if err != nil {
		fmt.Println(err)
		return
	}
	res, err := acct.Login(context.Background(), chatgpt.LoginOptions{Browser: server.browse, Say: io.Discard})
	if err != nil {
		fmt.Println(err)
		return
	}
	p, err := llm.Resolve("chatgpt", llm.ResolveOpts{TokenSource: acct})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(res.Email, p.Name())
	// Output: person@example.com chatgpt
}
