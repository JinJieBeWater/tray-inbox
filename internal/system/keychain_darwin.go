package system

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var runSecurity = func(stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command("/usr/bin/security", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.Output()
}

// security -i splits lines longer than 4095 bytes, newline included, into separate commands.
const securityLineMax = 4094

// security exits with errSecItemNotFound's low byte when the item is missing.
const securityNotFound = 44

func (k Keychain) Get(account string) (string, error) {
	if err := k.check(account); err != nil {
		return "", err
	}
	args := []string{"find-generic-password", "-a", account, "-s", k.Service, "-w"}
	if k.File != "" {
		args = append(args, k.File)
	}
	out, err := runSecurity("", args...)
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == securityNotFound:
		return "", ErrNotFound
	case exit != nil:
		return "", fmt.Errorf("keychain get: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
	case err != nil:
		return "", fmt.Errorf("keychain get: %w", err)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// Set stores the secret through security's stdin so it never shows up in argv.
// Secrets are limited to printable ASCII: find-generic-password -w prints anything
// else as hex, which Get could not tell apart from a real hex secret.
func (k Keychain) Set(account, secret string) error {
	if err := k.check(account); err != nil {
		return err
	}
	if secret == "" || strings.IndexFunc(secret, func(r rune) bool { return r < 0x20 || r > 0x7e }) >= 0 {
		return errors.New("keychain set: the secret must be non-empty printable ASCII")
	}
	line := "add-generic-password -U -a " + quote(account) + " -s " + quote(k.Service) +
		" -X " + hex.EncodeToString([]byte(secret))
	if k.File != "" {
		line += " " + quote(k.File)
	}
	if len(line) > securityLineMax {
		return errors.New("keychain set: the secret is too long")
	}
	if _, err := runSecurity(line+"\n", "-i"); err != nil {
		// security's stderr can echo the command line, secret included, so it is dropped.
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return fmt.Errorf("keychain set: security exited %d", exit.ExitCode())
		}
		return fmt.Errorf("keychain set: %w", err)
	}
	return nil
}

func (k Keychain) check(account string) error {
	if account == "" || k.Service == "" {
		return errors.New("keychain: account and service must be set")
	}
	for _, s := range []string{account, k.Service, k.File} {
		if strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return fmt.Errorf("keychain: control character in %q", s)
		}
	}
	if k.File == "" {
		return nil
	}
	// security silently uses the default keychain when the named file is missing.
	if !filepath.IsAbs(k.File) {
		return fmt.Errorf("keychain: %s is not an absolute path", k.File)
	}
	if fi, err := os.Stat(k.File); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("keychain: %s is not a keychain file", k.File)
	}
	return nil
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
