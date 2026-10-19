package chatgpt

import (
	"context"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"
)

// memStore is a Store with no file behind it, the shape of one an app keeps in
// a keychain.
type memStore struct {
	mu     sync.Mutex
	record []byte
	hostID string
	held   bool
	locks  int
}

var _ Store = (*memStore)(nil)

func (m *memStore) Ensure() error { return nil }

func (m *memStore) Has() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.record != nil
}

func (m *memStore) Location() string { return "memory" }

func (m *memStore) LoadRecord() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.record == nil {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), m.record...), nil
}

func (m *memStore) SaveRecord(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.record = append([]byte(nil), data...)
	return nil
}

func (m *memStore) LoadHostID() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hostID == "" {
		return "", fs.ErrNotExist
	}
	return m.hostID, nil
}

func (m *memStore) SaveHostID(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostID = id
	return nil
}

func (m *memStore) TryLock() (func(), bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.held {
		return nil, false, nil
	}
	m.held = true
	m.locks++
	return func() {
		m.mu.Lock()
		m.held = false
		m.mu.Unlock()
	}, true, nil
}

// An Account over a store that is not a FileStore signs in, refreshes and signs
// out against the fake authorization server, and leaves the lock released.
func TestStore_AnotherKindOfStoreSignsInRefreshesAndOut(t *testing.T) {
	clock := newClock()
	fake, client := newFakeAuth(t, clock)
	store := &memStore{}
	if HasRecord(store) {
		t.Fatal("an empty store holds a record")
	}
	a, err := Open(Options{App: testApp, Store: store, Client: client, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	a.lockPoll = 2 * time.Millisecond

	if _, res, err := signIn(t, a, fake, nil, nil); err != nil || !res.PlanGranted {
		t.Fatalf("Login = %+v, %v", res, err)
	}
	if !HasRecord(store) || store.hostID == "" || !strings.Contains(string(store.record), testSubject) {
		t.Fatalf("the login left record %q host ID %q", store.record, store.hostID)
	}
	signedIn, _ := a.Credential()

	clock.Advance(time.Hour)
	token, err := a.Token(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	refreshed, _ := a.Credential()
	if token == signedIn.AccessToken || refreshed.AccessToken != token || refreshed.RefreshToken == signedIn.RefreshToken {
		t.Errorf("the refresh left access %q refresh %q", refreshed.AccessToken, refreshed.RefreshToken)
	}

	revoked, err := a.Logout(context.Background())
	if err != nil || !revoked {
		t.Fatalf("Logout = %v, %v", revoked, err)
	}
	if c, _ := a.Credential(); c.SignedIn() || c.AccessToken != "" || c.HostID != signedIn.HostID {
		t.Errorf("after Logout: %+v", c)
	}
	if store.locks == 0 || store.held {
		t.Errorf("lock taken %d times, still held %v", store.locks, store.held)
	}
}

// A lock the store says is taken is waited for until the account's deadline.
func TestStore_AHeldLockTimesOut(t *testing.T) {
	store := &memStore{held: true}
	a, err := Open(Options{App: testApp, Store: store})
	if err != nil {
		t.Fatal(err)
	}
	a.lockWait, a.lockPoll = 10*time.Millisecond, time.Millisecond
	if _, err := a.HostID(); err == nil || !strings.Contains(err.Error(), "try again") {
		t.Fatalf("HostID with the lock held = %v", err)
	}
}
