package chatgpt

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rfizzle/llm"
)

// preStoreRecord is the exact text the code before FileStore wrote for the
// credential below (a whole-file replace of MarshalIndent plus a newline),
// captured from that code. It is the on-disk format the store must keep.
const preStoreRecord = "{\n  \"email\": \"a@b.c\",\n  \"issuer\": \"https://auth.openai.com\",\n  \"subject\": \"sub1\",\n  \"client_id\": \"cl1\",\n  \"host_id\": \"urn:uuid:11111111-2222-4333-8444-555555555555\",\n  \"id_token\": \"id.tok.sig\",\n  \"access_token\": \"acc.tok.sig\",\n  \"refresh_token\": \"rt_refresh\",\n  \"token_type\": \"Bearer\",\n  \"expires_at\": \"2026-10-18T13:00:00Z\",\n  \"earliest_refresh_at\": \"2026-10-18T12:30:00Z\",\n  \"scopes\": [\n    \"openid\",\n    \"chatgpt.tokens.use.direct\"\n  ],\n  \"saved_at\": \"2026-10-18T12:00:00Z\",\n  \"welcomed_at\": \"2026-10-18T12:00:00Z\"\n}\n"

const preStoreHostID = "urn:uuid:11111111-2222-4333-8444-555555555555"

func preStoreCredential() Credential {
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	return Credential{
		Email: "a@b.c", Issuer: Issuer, Subject: "sub1", ClientID: "cl1", HostID: preStoreHostID,
		IDToken: "id.tok.sig", AccessToken: "acc.tok.sig", RefreshToken: "rt_refresh", TokenType: "Bearer",
		ExpiresAt: at.Add(time.Hour), EarliestRefreshAt: at.Add(30 * time.Minute),
		Scopes: []string{"openid", PlanScope}, SavedAt: at, WelcomedAt: at,
	}
}

// A record and a host ID written before the store existed are read after it,
// and the store writes the same bytes the old code did, so the old code reads
// what the new one writes: the format and the file names did not change.
func TestFileStore_KeepsTheOnDiskFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(preStoreRecord), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "host-id"), []byte(preStoreHostID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := Open(Options{Store: FileStore{Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Credential()
	if err != nil {
		t.Fatal(err)
	}
	want := preStoreCredential()
	if got.RefreshToken != want.RefreshToken || got.ClientID != want.ClientID || !got.ExpiresAt.Equal(want.ExpiresAt) || !got.WelcomedAt.Equal(want.WelcomedAt) || !got.PlanGranted() {
		t.Fatalf("the record written before the store was misread: %+v", got)
	}
	if id, err := a.HostID(); err != nil || id != preStoreHostID {
		t.Fatalf("host ID = %q, %v", id, err)
	}

	// The reverse: a fresh directory written by the store holds, byte for
	// byte, what the old code wrote.
	fresh := t.TempDir()
	b, err := Open(Options{Store: FileStore{Dir: fresh}})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.write(want); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(fresh, "credentials.json"))
	if err != nil || string(data) != preStoreRecord {
		t.Fatalf("the store wrote %q, %v; want the pre-store bytes", data, err)
	}
	if err := b.store.writeHostID(preStoreHostID); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(fresh, "host-id")); string(data) != preStoreHostID+"\n" {
		t.Fatalf("host-id = %q", data)
	}
	for _, name := range []string{"credentials.json", "host-id"} {
		if info, err := os.Stat(filepath.Join(fresh, name)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
}

func TestOpen_NeedsAStoreDirectory(t *testing.T) {
	if _, err := Open(Options{}); err == nil {
		t.Fatal("an account with no store directory opened")
	}
	if HasRecord(FileStore{}) {
		t.Fatal("an empty store holds a record")
	}
}

// A store with no directory answers every operation the same way, so the
// guard does not depend on Open having refused it first.
func TestFileStore_NoDirectoryIsRefusedEverywhere(t *testing.T) {
	var s FileStore
	if _, err := s.readCredential(); !errors.Is(err, errNoDir) {
		t.Errorf("readCredential = %v", err)
	}
	if _, err := s.readHostID(); !errors.Is(err, errNoDir) {
		t.Errorf("readHostID = %v", err)
	}
	if err := s.writeHostID("urn:uuid:x"); !errors.Is(err, errNoDir) {
		t.Errorf("writeHostID = %v", err)
	}
	if f, err := s.openLock(); !errors.Is(err, errNoDir) || f != nil {
		t.Errorf("openLock = %v, %v", f, err)
	}
}

// A write that fails part-way leaves the old record whole and no temporary
// file behind, whichever step failed.
func TestReplaceFile_AFailureLeavesTheOldFileWhole(t *testing.T) {
	failed := errors.New("disk said no")
	for name, fail := range map[string]func(){
		"sync":   func() { syncFile = func(*os.File) error { return failed } },
		"rename": func() { renameFile = func(string, string) error { return failed } },
	} {
		t.Run(name, func(t *testing.T) {
			savedSync, savedRename := syncFile, renameFile
			t.Cleanup(func() { syncFile, renameFile = savedSync, savedRename })
			dir := t.TempDir()
			path := filepath.Join(dir, "credentials.json")
			if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			fail()
			if err := replaceFile(path, "new\n", 0o600); !errors.Is(err, failed) {
				t.Fatalf("replaceFile = %v, want the disk's error", err)
			}
			if got, _ := os.ReadFile(path); string(got) != "old\n" {
				t.Errorf("the record = %q, want the old one whole", got)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Errorf("a temporary file was left behind: %v", entries)
			}
		})
	}
}

// The directory is synced after the rename, and a directory that will not
// sync is not a failed write.
func TestReplaceFile_SyncsTheDirectoryAfterTheRename(t *testing.T) {
	savedRename, savedDir := renameFile, syncDir
	t.Cleanup(func() { renameFile, syncDir = savedRename, savedDir })
	var order []string
	renameFile = func(from, to string) error { order = append(order, "rename"); return savedRename(from, to) }
	syncDir = func(string) error { order = append(order, "sync dir"); return errors.New("not supported") }
	path := filepath.Join(t.TempDir(), "host-id")
	if err := replaceFile(path, "a\n", 0o600); err != nil {
		t.Fatalf("a directory that will not sync failed the write: %v", err)
	}
	if got := strings.Join(order, ","); got != "rename,sync dir" {
		t.Errorf("order = %s", got)
	}
}

// The app names itself: the expired sign-in carries its command, and the
// bare sentinel still matches.
func TestSignInRequired_NamesTheAppsCommand(t *testing.T) {
	err := error(signInExpired{testApp.SignInHint})
	if want := "the ChatGPT sign-in has expired \u2014 run `acme login chatgpt`"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if !errors.Is(err, ErrSignInRequired) || !errors.Is(err, llm.ErrChatGPTSignedOut) {
		t.Errorf("%v does not match ErrSignInRequired and the signed-out error", err)
	}
	if got := ErrSignInRequired.Error(); got != "the ChatGPT sign-in has expired" {
		t.Errorf("sentinel = %q", got)
	}
}

// The pages the browser lands on call the program by the name the app gave.
func TestCallbackPages_NameTheApp(t *testing.T) {
	h := newCallbackHandler("127.0.0.1:1", "st", "acme")
	for query, want := range map[string]string{
		"state=bad":                   "This is not the sign-in acme is waiting for.\n",
		"state=st&code=c&client_id=x": "acme has the sign-in. You can close this tab; the terminal says how it went.\n",
	} {
		req := httptest.NewRequest(http.MethodGet, "/auth/callback?"+query, nil)
		req.Host = "127.0.0.1:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Body.String(); got != want {
			t.Errorf("page for %s = %q, want %q", query, got, want)
		}
	}
}
