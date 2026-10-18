package chatgpt_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/rfizzle/llm/chatgpt"
	"github.com/rfizzle/llm/internal/testhttp"
)

// exampleAuth is auth.openai.com in memory, enough for one sign-in: the
// discovery document, a key set, and a token endpoint that issues an ID token
// for person@example.com. It is reached under the real host name through
// testhttp, so no request leaves the process. The examples in this package
// use it as setup; it is not part of the API.
type exampleAuth struct {
	key *ecdsa.PrivateKey
	mu  sync.Mutex
	// nonce is what the last authorize URL asked for, which the ID token
	// carries back.
	nonce string
}

// newExampleAuth returns the fake and an http.Client that reaches it.
func newExampleAuth() (*exampleAuth, *http.Client) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	f := &exampleAuth{key: key}
	reg := &testhttp.Registry{}
	reg.NewServer(f).Alias("auth.openai.com")
	return f, reg.Client()
}

// browse plays the person's browser. It is the Browser of a LoginOptions: it
// reads the authorize URL, tells the fake the nonce, and calls the loopback
// callback back as a person who approved.
func (f *exampleAuth) browse(page string) error {
	u, err := url.Parse(page)
	if err != nil {
		return err
	}
	q := u.Query()
	f.mu.Lock()
	f.nonce = q.Get("nonce")
	f.mu.Unlock()
	back := url.Values{"state": {q.Get("state")}, "code": {"auth-code"}, "client_id": {"oaiapp_example"}}
	go func() {
		resp, err := http.Get(q.Get("redirect_uri") + "?" + back.Encode())
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	return nil
}

func (f *exampleAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":              chatgpt.Issuer,
			"jwks_uri":            chatgpt.Issuer + "/jwks",
			"revocation_endpoint": chatgpt.Issuer + "/oauth/revoke",
		})
	case "/jwks":
		raw, _ := f.key.PublicKey.Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "EC", "crv": "P-256", "kid": "k1", "alg": "ES256", "use": "sig",
			"x": base64.RawURLEncoding.EncodeToString(raw[1:33]),
			"y": base64.RawURLEncoding.EncodeToString(raw[33:]),
		}}})
	case "/api/accounts/oauth/token":
		_ = r.ParseForm()
		f.mu.Lock()
		nonce := f.nonce
		f.mu.Unlock()
		now := time.Now()
		id := f.sign(map[string]any{
			"iss": chatgpt.Issuer, "sub": "user-abc123", "aud": r.PostForm.Get("client_id"), "email": "person@example.com",
			"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(), "nonce": nonce,
		})
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at_example", "refresh_token": "rt_example", "id_token": id,
			"token_type": "Bearer", "expires_in": 3600, "scope": chatgpt.Scopes,
		})
	default:
		http.NotFound(w, r)
	}
}

// sign mints an ES256 JWT over claims.
func (f *exampleAuth) sign(claims map[string]any) string {
	part := func(v any) string {
		data, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(data)
	}
	signed := part(map[string]string{"alg": "ES256", "kid": "k1", "typ": "JWT"}) + "." + part(claims)
	digest := sha256.Sum256([]byte(signed))
	r, s, err := ecdsa.Sign(rand.Reader, f.key, digest[:])
	if err != nil {
		panic(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}
