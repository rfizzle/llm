package credstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// File names inside the account directory.
const (
	credentialsFile = "credentials.json"
	hostIDFile      = "host-id"
	lockFileName    = "credentials.lock"
)

// errNoDir is what a store with no directory answers to every operation.
var errNoDir = errors.New("chatgpt: the store has no directory")

// Store is where an Account keeps its credential record and host ID, and the
// lock that serialises refreshes. An app whose secrets live somewhere other
// than a directory, a keychain for one, implements it; FileStore is the
// directory-backed one.
//
// The lock is one method, TryLock, and not a blocking Lock, because the wait
// is the Account's: it polls, gives up after its own deadline and says so in
// its own words, so a store only answers whether it could take the lock now.
// A store with one process behind it can answer with a mutex.
//
// Every method may be called from several goroutines at once. Errors are
// returned as they are, except that LoadRecord and LoadHostID answer an absent
// value with an error that satisfies errors.Is(err, fs.ErrNotExist).
type Store interface {
	// Ensure prepares the store for use, creating whatever it keeps things
	// in. Open calls it; it must be safe to call again.
	Ensure() error
	// Has reports whether a record is stored, creating nothing.
	Has() bool
	// Location names where the record is, for a sentence in an error.
	Location() string
	// LoadRecord is the stored record's bytes.
	LoadRecord() ([]byte, error)
	// SaveRecord replaces the record whole: a reader sees the old record or
	// the new one, never a part of either.
	SaveRecord(data []byte) error
	// LoadHostID is the stored host ID, trimmed of surrounding space.
	LoadHostID() (string, error)
	// SaveHostID stores the host ID.
	SaveHostID(id string) error
	// TryLock takes the exclusive lock without waiting. When ok is true the
	// caller holds it until it calls unlock; when ok is false nothing is held
	// and unlock is nil. The lock has to exclude every other Account that
	// shares the record, in this process and in any other that can reach it.
	TryLock() (unlock func(), ok bool, err error)
}

// FileStore keeps the credential record, the host ID and the lock file in one
// directory the app names. The record's whole-file replace and the
// cross-process lock are file operations here. The directory is created 0700
// and every file in it is 0600.
type FileStore struct {
	// Dir is the account directory.
	Dir string
}

// CredentialsPath is the record's path.
func (s FileStore) CredentialsPath() string { return filepath.Join(s.Dir, credentialsFile) }

func (s FileStore) hostIDPath() string { return filepath.Join(s.Dir, hostIDFile) }

// Location is the record's path.
func (s FileStore) Location() string { return s.CredentialsPath() }

// Ensure creates the directory owner-only. A directory that was already there
// keeps nobody else's read on it: the record inside is a credential.
func (s FileStore) Ensure() error {
	if s.Dir == "" {
		return errNoDir
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(s.Dir, 0o700)
}

// Has reports whether the directory holds a credential record, creating
// nothing.
func (s FileStore) Has() bool {
	if s.Dir == "" {
		return false
	}
	_, err := os.Stat(s.CredentialsPath())
	return err == nil
}

// LoadRecord is the record's bytes; a record not there is fs.ErrNotExist.
func (s FileStore) LoadRecord() ([]byte, error) {
	if s.Dir == "" {
		return nil, errNoDir
	}
	return os.ReadFile(s.CredentialsPath())
}

// SaveRecord replaces the record whole and owner-only. replaceFile keeps
// the mode of a file already there, so a record copied in with a looser mode
// is narrowed first rather than rewritten as readable as it arrived.
func (s FileStore) SaveRecord(data []byte) error {
	path := s.CredentialsPath()
	if info, err := os.Stat(path); err == nil && info.Mode().Perm() != 0o600 {
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
	}
	return replaceFile(path, string(data), 0o600)
}

// LoadHostID is the host-id file's text, trimmed; one not there is
// fs.ErrNotExist.
func (s FileStore) LoadHostID() (string, error) {
	if s.Dir == "" {
		return "", errNoDir
	}
	data, err := os.ReadFile(s.hostIDPath())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SaveHostID writes the host-id file.
func (s FileStore) SaveHostID(id string) error {
	if s.Dir == "" {
		return errNoDir
	}
	return replaceFile(s.hostIDPath(), id+"\n", 0o600)
}

// openLock opens credentials.lock, a file of its own because the record is
// replaced by a rename: a lock held on the record would be on the old file the
// moment the first writer renamed over it.
func (s FileStore) openLock() (*os.File, error) {
	if s.Dir == "" {
		return nil, errNoDir
	}
	return os.OpenFile(filepath.Join(s.Dir, lockFileName), os.O_RDWR|os.O_CREATE, 0o600)
}

// TryLock takes an advisory lock on credentials.lock without waiting.
func (s FileStore) TryLock() (func(), bool, error) {
	f, err := s.openLock()
	if err != nil {
		return nil, false, err
	}
	ok, err := lockFile(f)
	if err != nil || !ok {
		f.Close()
		return nil, false, err
	}
	return func() { unlockFile(f); f.Close() }, true, nil
}

// replaceFile writes text to path so a reader sees the old file whole or the
// new one whole, never a prefix of either: the text goes to a temporary file
// in the same directory, which is synced before it is renamed over the path,
// and the directory is synced after the rename. A file already there keeps its
// own mode; mode is for a new one.
func replaceFile(path, text string, mode os.FileMode) error {
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.WriteString(text)
	if werr == nil {
		werr = syncFile(tmp)
	}
	if err := tmp.Close(); werr == nil {
		werr = err
	}
	if werr == nil {
		werr = os.Chmod(name, mode)
	}
	if werr == nil {
		werr = renameFile(name, path)
	}
	if werr != nil {
		_ = os.Remove(name) // the write is the failure to report
		return werr
	}
	// The file is in place by now; a directory the platform will not sync —
	// Windows opens none for writing — is not a failed write.
	_ = syncDir(dir)
	return nil
}

// The three steps of replaceFile a test replaces, to fail one part-way or to
// read the order they ran in. They are process-global: a test that swaps one
// changes every store in the process, so it must not call t.Parallel, nor run
// beside a test that does.
var (
	syncFile   = func(f *os.File) error { return f.Sync() }
	renameFile = os.Rename
	syncDir    = func(dir string) error {
		d, err := os.Open(dir)
		if err != nil {
			return err
		}
		serr := d.Sync()
		if err := d.Close(); serr == nil {
			serr = err
		}
		return serr
	}
)
