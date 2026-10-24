// Package accounts resolves bot account declarations and plans the legacy
// data migration. Resolve is a pure function over the WA_ACCOUNTS value and
// filesystem state so it can be tested without touching the disk.
package accounts

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// slugPattern matches account names that are safe for URLs and file paths.
var slugPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// DefaultAccountName is the account name used when WA_ACCOUNTS is unset.
const DefaultAccountName = "default"

// DefaultDir is the migration target directory for legacy root data files.
const DefaultDir = "data/" + DefaultAccountName

// LegacyFiles are the legacy root-level data file names that may need to move
// into data/default on first boot. The legacy backup database
// (pm-wa-server.db) is deliberately not part of this list and is never moved.
var legacyFiles = []string{
	"pm-wa.db", "pm-wa.db-wal", "pm-wa.db-shm",
	"wa-session.db", "wa-session.db-wal", "wa-session.db-shm",
}

// LegacyFiles returns the legacy root data file names that may be migrated.
// Callers stat these paths to build the FSState input for Resolve.
func LegacyFiles() []string {
	return append([]string(nil), legacyFiles...)
}

// Account is one resolved bot account with its isolated data paths.
type Account struct {
	// Name is the validated account slug.
	Name string
	// DataDir is the account's data directory ("data/<name>").
	DataDir string
	// AppDBPath is the account's task database path.
	AppDBPath string
	// SessionDBPath is the account's WhatsApp session store path.
	SessionDBPath string
}

// FSState is the filesystem input to Resolve: which legacy root data files
// exist and whether the default account directory already exists.
type FSState struct {
	// LegacyFiles maps each name from LegacyFiles() to whether the file
	// exists at the repo root. Missing keys count as nonexistent.
	LegacyFiles map[string]bool
	// DefaultDirExists reports whether data/default already exists.
	DefaultDirExists bool
}

// Resolution is the output of Resolve.
type Resolution struct {
	// Accounts is the resolved account list, in declaration order.
	Accounts []Account
	// MoveToDefault lists legacy file names (from LegacyFiles()) that should
	// be moved into DefaultDir before anything connects. Empty when no
	// migration is needed.
	MoveToDefault []string
	// DefaultDir is the migration target directory ("data/default"). Only
	// meaningful when MoveToDefault is non-empty.
	DefaultDir string
}

// Validate checks an account name against the URL-safe slug rules.
func Validate(name string) error {
	if !slugPattern.MatchString(name) {
		return fmt.Errorf("invalid account name %q: must match [a-z0-9-]+", name)
	}
	return nil
}

// Resolve parses the WA_ACCOUNTS value against the given filesystem state.
// An empty value resolves to the single default account. Resolve does not
// touch the filesystem; the caller performs the moves it reports.
func Resolve(envValue string, st FSState) (Resolution, error) {
	names, err := parseNames(envValue)
	if err != nil {
		return Resolution{}, err
	}

	res := Resolution{DefaultDir: DefaultDir}
	for _, name := range names {
		res.Accounts = append(res.Accounts, Account{
			Name:          name,
			DataDir:       "data/" + name,
			AppDBPath:     "data/" + name + "/pm-wa.db",
			SessionDBPath: "data/" + name + "/wa-session.db",
		})
	}

	if !st.DefaultDirExists {
		for _, file := range legacyFiles {
			if st.LegacyFiles[file] {
				res.MoveToDefault = append(res.MoveToDefault, file)
			}
		}
	}
	return res, nil
}

// parseNames splits and validates the WA_ACCOUNTS value. An empty value
// resolves to the single default account.
func parseNames(envValue string) ([]string, error) {
	if strings.TrimSpace(envValue) == "" {
		return []string{DefaultAccountName}, nil
	}

	seen := make(map[string]bool)
	var names []string
	for _, raw := range strings.Split(envValue, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, fmt.Errorf("invalid WA_ACCOUNTS %q: empty account name", envValue)
		}
		if err := Validate(name); err != nil {
			return nil, fmt.Errorf("invalid WA_ACCOUNTS %q: %w", envValue, err)
		}
		if seen[name] {
			return nil, fmt.Errorf("invalid WA_ACCOUNTS %q: duplicate account name %q", envValue, name)
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, nil
}

// StatFSState builds the FSState inputs by looking at the current working
// directory: the legacy root data files and the default account directory.
// This is the thin, untested glue between the disk and the pure Resolve.
func StatFSState() FSState {
	st := FSState{LegacyFiles: make(map[string]bool, len(legacyFiles))}
	for _, file := range legacyFiles {
		st.LegacyFiles[file] = pathExists(file)
	}
	st.DefaultDirExists = pathExists(DefaultDir)
	return st
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// EnsureDir creates the account's data directory if missing. Thin OS glue.
func (a Account) EnsureDir() error {
	return os.MkdirAll(a.DataDir, 0o755)
}

// Apply performs the one-time migration planned by Resolve: it creates the
// default account directory and moves the listed legacy files into it.
// Thin OS glue (os.Rename), per the epic's testing decisions.
func Apply(res Resolution) error {
	if len(res.MoveToDefault) == 0 {
		return nil
	}
	if err := os.MkdirAll(res.DefaultDir, 0o755); err != nil {
		return err
	}
	for _, file := range res.MoveToDefault {
		dst := res.DefaultDir + "/" + file
		if err := os.Rename(file, dst); err != nil {
			return fmt.Errorf("failed to move legacy file %s: %w", file, err)
		}
	}
	return nil
}
