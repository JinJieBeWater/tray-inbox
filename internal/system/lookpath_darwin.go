package system

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// LookPath finds a command the way the user's login shell would. An app launched
// from Finder inherits launchd's PATH, not the one set up in the shell profile.
func LookPath(name string) (string, error) {
	return paths.look(name)
}

var paths = newLookup(loginShell)

var safeName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type lookup struct {
	run func(shell, script string) ([]byte, error)

	mu    sync.Mutex
	names map[string]*lookupResult
}

type lookupResult struct {
	once sync.Once
	path string
	err  error
}

func newLookup(run func(shell, script string) ([]byte, error)) *lookup {
	return &lookup{run: run, names: map[string]*lookupResult{}}
}

func (l *lookup) look(name string) (string, error) {
	if !safeName.MatchString(name) {
		return "", fmt.Errorf("look up %q: unsafe command name", name)
	}
	l.mu.Lock()
	r, ok := l.names[name]
	if !ok {
		r = &lookupResult{}
		l.names[name] = r
	}
	l.mu.Unlock()
	r.once.Do(func() { r.path, r.err = l.resolve(name) })
	return r.path, r.err
}

func (l *lookup) resolve(name string) (string, error) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/zsh"
	}
	out, err := l.run(sh, "command -v "+name)
	if err != nil {
		return "", fmt.Errorf("look up %s: %w", name, err)
	}
	// Profiles may print banners, so only the last line is command -v's answer.
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	p := string(bytes.TrimSpace(lines[len(lines)-1]))
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("look up %s: %w", name, ErrNotFound)
	}
	if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("look up %s: %s is not an executable file", name, p)
	}
	return p, nil
}

func loginShell(shell, script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, shell, "-l", "-c", script).Output()
}
