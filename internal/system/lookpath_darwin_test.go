package system

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func executable(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t3")
	if err := os.WriteFile(p, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLookPathUsesLoginShellOutput(t *testing.T) {
	t3 := executable(t)
	t.Setenv("SHELL", "/bin/bash")
	var gotShell, gotScript string
	l := newLookup(func(shell, script string) ([]byte, error) {
		gotShell, gotScript = shell, script
		return []byte("Welcome to your profile\n" + t3 + "\n"), nil
	})
	got, err := l.look("t3")
	if err != nil {
		t.Fatal(err)
	}
	if got != t3 {
		t.Fatalf("path = %q, want %q", got, t3)
	}
	if gotShell != "/bin/bash" || gotScript != "command -v t3" {
		t.Fatalf("ran %q -l -c %q", gotShell, gotScript)
	}
}

func TestLookPathDefaultsToZsh(t *testing.T) {
	t3 := executable(t)
	t.Setenv("SHELL", "")
	var gotShell string
	l := newLookup(func(shell, script string) ([]byte, error) {
		gotShell = shell
		return []byte(t3 + "\n"), nil
	})
	if _, err := l.look("t3"); err != nil {
		t.Fatal(err)
	}
	if gotShell != "/bin/zsh" {
		t.Fatalf("shell = %q", gotShell)
	}
}

func TestLookPathNotFound(t *testing.T) {
	notExec := filepath.Join(t.TempDir(), "t3")
	if err := os.WriteFile(notExec, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]struct {
		b   []byte
		err error
	}{
		"shell fails":    {nil, errors.New("exit status 1")},
		"alias":          {[]byte("t3='t3 --x'\n"), nil},
		"builtin":        {[]byte("t3\n"), nil},
		"empty":          {nil, nil},
		"not executable": {[]byte(notExec + "\n"), nil},
		"missing file":   {[]byte("/nonexistent/t3\n"), nil},
	} {
		t.Run(name, func(t *testing.T) {
			l := newLookup(func(string, string) ([]byte, error) { return out.b, out.err })
			if p, err := l.look("t3"); err == nil {
				t.Fatalf("found %q", p)
			}
		})
	}
}

func TestLookPathRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"", "t3;rm", "a b", "$(x)", "../t3", "h\nc"} {
		l := newLookup(func(string, string) ([]byte, error) {
			t.Fatalf("ran the shell for %q", name)
			return nil, nil
		})
		if _, err := l.look(name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func TestLookPathRunsOncePerName(t *testing.T) {
	t3 := executable(t)
	var runs atomic.Int32
	release := make(chan struct{})
	l := newLookup(func(string, string) ([]byte, error) {
		runs.Add(1)
		<-release
		return []byte(t3 + "\n"), nil
	})
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for range 8 {
		wg.Go(func() {
			p, err := l.look("t3")
			if err != nil {
				t.Error(err)
			}
			results <- p
		})
	}
	close(release)
	wg.Wait()
	close(results)
	for p := range results {
		if p != t3 {
			t.Fatalf("path = %q", p)
		}
	}
	if n := runs.Load(); n != 1 {
		t.Fatalf("shell ran %d times", n)
	}
	if _, err := l.look("t3"); err != nil || runs.Load() != 1 {
		t.Fatalf("cached lookup: err=%v runs=%d", err, runs.Load())
	}
}

func TestLookPathFindsShInRealLoginShell(t *testing.T) {
	p, err := LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	if p != "/bin/sh" {
		t.Fatalf("sh = %q", p)
	}
}
