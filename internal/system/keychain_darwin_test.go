package system

import (
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tempKeychain makes a throwaway keychain file. It is not added to the search list,
// and every Keychain call below names it explicitly.
func tempKeychain(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "trayinbox-test.keychain")
	security := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("/usr/bin/security", args...).CombinedOutput(); err != nil {
			t.Fatalf("security %s: %v\n%s", args[0], err, out)
		}
	}
	security("create-keychain", "-p", "trayinbox-test", file)
	t.Cleanup(func() { security("delete-keychain", file) })
	security("unlock-keychain", "-p", "trayinbox-test", file)
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("keychain file was not created: %v", err)
	}
	return file
}

func recordSecurity(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	real := runSecurity
	runSecurity = func(stdin string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return real(stdin, args...)
	}
	t.Cleanup(func() { runSecurity = real })
	return &calls
}

func forbidSecurity(t *testing.T) {
	t.Helper()
	real := runSecurity
	runSecurity = func(stdin string, args ...string) ([]byte, error) {
		t.Fatalf("ran security %v", args)
		return nil, nil
	}
	t.Cleanup(func() { runSecurity = real })
}

func TestKeychainGetMissing(t *testing.T) {
	k := Keychain{Service: "trayinbox-test", File: tempKeychain(t)}
	if _, err := k.Get("nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestKeychainSetThenGet(t *testing.T) {
	file := tempKeychain(t)
	k := Keychain{Service: `trayinbox "test" \ service`, File: file}
	account := `env 1 "a" 'b' \c`
	secret := `t0k en"with'quotes\and~` + strings.Repeat("x", 1500)
	calls := recordSecurity(t)

	if err := k.Set(account, secret); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Get(account); err != nil || got != secret {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := k.Set(account, "second"); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Get(account); err != nil || got != "second" {
		t.Fatalf("Get after update = %q, %v", got, err)
	}
	other := Keychain{Service: "trayinbox-other", File: file}
	if _, err := other.Get(account); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other service: err = %v", err)
	}

	for _, args := range *calls {
		joined := strings.Join(args, " ")
		for _, s := range []string{secret, "second"} {
			if strings.Contains(joined, s) || strings.Contains(joined, hex.EncodeToString([]byte(s))) {
				t.Fatalf("secret in argv: security %s", joined)
			}
		}
	}
}

func TestKeychainNeverFallsBackToTheDefaultKeychain(t *testing.T) {
	forbidSecurity(t)
	for _, file := range []string{filepath.Join(t.TempDir(), "missing.keychain"), "relative.keychain", t.TempDir()} {
		k := Keychain{Service: "trayinbox-test", File: file}
		if err := k.Set("a", "s"); err == nil {
			t.Fatalf("Set into %q succeeded", file)
		}
		if _, err := k.Get("a"); err == nil || errors.Is(err, ErrNotFound) {
			t.Fatalf("Get from %q: err = %v", file, err)
		}
	}
}

func TestKeychainRejectsInputItCannotPassSafely(t *testing.T) {
	file := filepath.Join(t.TempDir(), "unused.keychain")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	forbidSecurity(t)
	ok := Keychain{Service: "trayinbox-test", File: file}
	for name, set := range map[string]func() error{
		"newline in account":  func() error { return ok.Set("a\nb", "s") },
		"newline in service":  func() error { return Keychain{Service: "s\n", File: file}.Set("a", "s") },
		"empty service":       func() error { return Keychain{File: file}.Set("a", "s") },
		"empty account":       func() error { return ok.Set("", "s") },
		"empty secret":        func() error { return ok.Set("a", "") },
		"non-ASCII secret":    func() error { return ok.Set("a", "é") },
		"control char secret": func() error { return ok.Set("a", "a\tb") },
		"too long":            func() error { return ok.Set("a", strings.Repeat("x", 2100)) },
	} {
		if err := set(); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
	if _, err := ok.Get("a\nb"); err == nil {
		t.Fatal("Get accepted a newline in the account")
	}
}
