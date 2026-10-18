package chatgpt_test

import (
	"context"
	"fmt"
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
// spends the plan afterwards. It needs a browser and the sign-in server, so
// it is compiled and not run.
func Example_signIn() {
	acct, err := chatgpt.Open(chatgpt.Options{Store: chatgpt.FileStore{Dir: "/path/to/acme/chatgpt"}})
	if err != nil {
		fmt.Println(err)
		return
	}
	if _, err := acct.Login(context.Background(), chatgpt.LoginOptions{Say: os.Stdout}); err != nil {
		fmt.Println(err)
		return
	}
	p, err := llm.Resolve("chatgpt", llm.ResolveOpts{TokenSource: acct})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(p.Name())
}
