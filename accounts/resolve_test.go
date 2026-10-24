package accounts

import (
	"reflect"
	"strings"
	"testing"
)

const allLegacy = "pm-wa.db pm-wa.db-wal pm-wa.db-shm wa-session.db wa-session.db-wal wa-session.db-shm"

func legacyState(t *testing.T, present string, defaultDirExists bool) FSState {
	t.Helper()
	st := FSState{DefaultDirExists: defaultDirExists}
	files := map[string]bool{}
	// Seed with the known legacy files, but allow unknown extras (like the
	// legacy backup DB) so tests can assert they are never moved.
	for _, name := range LegacyFiles() {
		files[name] = false
	}
	for _, name := range strings.Fields(present) {
		files[name] = true
	}
	st.LegacyFiles = files
	return st
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		st          FSState
		wantNames   []string
		wantMoves   []string
		wantErr     bool
		wantErrHas  string
	}{
		{
			name:      "unset env resolves default account",
			env:       "",
			st:        legacyState(t, "", false),
			wantNames: []string{"default"},
		},
		{
			name:      "whitespace env resolves default account",
			env:       "   ",
			st:        legacyState(t, "", false),
			wantNames: []string{"default"},
		},
		{
			name:      "multiple names resolve to per-account paths",
			env:       "sales,service",
			st:        legacyState(t, "", false),
			wantNames: []string{"sales", "service"},
		},
		{
			name:      "spaces around names are trimmed",
			env:       "sales, service",
			st:        legacyState(t, "", false),
			wantNames: []string{"sales", "service"},
		},
		{
			name:      "hyphens and digits are valid slugs",
			env:       "bonus-2",
			st:        legacyState(t, "", false),
			wantNames: []string{"bonus-2"},
		},
		{
			name:       "uppercase name is invalid",
			env:        "Sales",
			st:         legacyState(t, "", false),
			wantErr:    true,
			wantErrHas: "Sales",
		},
		{
			name:       "name with space is invalid",
			env:        "sales team",
			st:         legacyState(t, "", false),
			wantErr:    true,
			wantErrHas: "sales team",
		},
		{
			name:       "name with underscore is invalid",
			env:        "sales_team",
			st:         legacyState(t, "", false),
			wantErr:    true,
		},
		{
			name:       "duplicate names are rejected",
			env:        "sales,sales",
			st:         legacyState(t, "", false),
			wantErr:    true,
			wantErrHas: "duplicate",
		},
		{
			name:       "duplicate after trimming is rejected",
			env:        "sales, sales",
			st:         legacyState(t, "", false),
			wantErr:    true,
			wantErrHas: "duplicate",
		},
		{
			name:       "empty token is rejected",
			env:        "sales,,service",
			st:         legacyState(t, "", false),
			wantErr:    true,
			wantErrHas: "empty",
		},
		{
			name:      "all legacy files present migrates into data/default",
			env:       "",
			st:        legacyState(t, allLegacy, false),
			wantNames: []string{"default"},
			wantMoves: strings.Fields(allLegacy),
		},
		{
			name:      "partial legacy files migrate individually",
			env:       "",
			st:        legacyState(t, "pm-wa.db pm-wa.db-wal", false),
			wantNames: []string{"default"},
			wantMoves: []string{"pm-wa.db", "pm-wa.db-wal"},
		},
		{
			name:      "legacy session db present migrates too",
			env:       "",
			st:        legacyState(t, "wa-session.db-shm", false),
			wantNames: []string{"default"},
			wantMoves: []string{"wa-session.db-shm"},
		},
		{
			name:      "existing data/default prevents migration",
			env:       "",
			st:        legacyState(t, allLegacy, true),
			wantNames: []string{"default"},
		},
		{
			name:      "no legacy files means no migration",
			env:       "",
			st:        legacyState(t, "", false),
			wantNames: []string{"default"},
		},
		{
			name:      "legacy backup db is never moved",
			env:       "",
			st:        legacyState(t, "pm-wa-server.db", false),
			wantNames: []string{"default"},
		},
		{
			name:      "explicit default in WA_ACCOUNTS resolves one account",
			env:       "default",
			st:        legacyState(t, "", false),
			wantNames: []string{"default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Resolve(tt.env, tt.st)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve() error = nil, want error")
				}
				if tt.wantErrHas != "" && !strings.Contains(err.Error(), tt.wantErrHas) {
					t.Errorf("Resolve() error = %q, want it to contain %q", err.Error(), tt.wantErrHas)
				}
				if res.Accounts != nil {
					t.Errorf("Resolve() on error should return no accounts, got %+v", res.Accounts)
				}
				return
			}

			if err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}

			gotNames := make([]string, 0, len(res.Accounts))
			for _, a := range res.Accounts {
				gotNames = append(gotNames, a.Name)
				wantDir := "data/" + a.Name
				if a.DataDir != wantDir {
					t.Errorf("Account %s DataDir = %q, want %q", a.Name, a.DataDir, wantDir)
				}
				if a.AppDBPath != wantDir+"/pm-wa.db" {
					t.Errorf("Account %s AppDBPath = %q, want %q", a.Name, a.AppDBPath, wantDir+"/pm-wa.db")
				}
				if a.SessionDBPath != wantDir+"/wa-session.db" {
					t.Errorf("Account %s SessionDBPath = %q, want %q", a.Name, a.SessionDBPath, wantDir+"/wa-session.db")
				}
			}
			if !reflect.DeepEqual(gotNames, tt.wantNames) {
				t.Fatalf("Resolve() accounts = %v, want %v", gotNames, tt.wantNames)
			}

			if !reflect.DeepEqual(res.MoveToDefault, tt.wantMoves) {
				t.Errorf("Resolve() MoveToDefault = %v, want %v", res.MoveToDefault, tt.wantMoves)
			}

			if len(res.MoveToDefault) > 0 && res.DefaultDir != "data/default" {
				t.Errorf("Resolve() DefaultDir = %q, want %q", res.DefaultDir, "data/default")
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "valid lowercase", in: "sales"},
		{name: "valid digits", in: "2"},
		{name: "valid hyphen", in: "bonus-2"},
		{name: "empty", in: "", wantErr: true},
		{name: "uppercase", in: "Sales", wantErr: true},
		{name: "space", in: "a b", wantErr: true},
		{name: "underscore", in: "a_b", wantErr: true},
		{name: "dot", in: "a.b", wantErr: true},
		{name: "slash", in: "a/b", wantErr: true},
		{name: "path traversal", in: "..", wantErr: true},
		{name: "emoji", in: "a🙂", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.in)
			if tt.wantErr && err == nil {
				t.Errorf("Validate(%q) = nil, want error", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate(%q) = %v, want nil", tt.in, err)
			}
		})
	}
}
