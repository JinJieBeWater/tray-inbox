//go:build darwin

package system

import (
	"context"
	"errors"
	"testing"
)

func TestAppBundle(t *testing.T) {
	for _, tc := range []struct {
		exec string
		want string
		ok   bool
	}{
		{"/Applications/T3 Code (Nightly).app/Contents/MacOS/T3 Code (Nightly)", "/Applications/T3 Code (Nightly).app", true},
		{"/Applications/T3 Code (Nightly).app/Contents/Frameworks/T3 Code (Nightly) Helper.app/Contents/MacOS/Helper", "/Applications/T3 Code (Nightly).app", true},
		{"/Users/dev/t3code/apps/desktop/.electron-runtime/T3 Code (Dev).app/Contents/MacOS/Electron", "/Users/dev/t3code/apps/desktop/.electron-runtime/T3 Code (Dev).app", true},
		{"/opt/homebrew/bin/node", "", false},
	} {
		got, ok := appBundle(tc.exec)
		if ok != tc.ok || got != tc.want {
			t.Errorf("appBundle(%q) = (%q, %v), want (%q, %v)", tc.exec, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParsePSLine(t *testing.T) {
	ppid, comm, ok := parsePSLine("  16903 /Applications/T3 Code (Nightly).app/Contents/MacOS/T3 Code (Nightly)\n")
	if !ok || ppid != 16903 || comm != "/Applications/T3 Code (Nightly).app/Contents/MacOS/T3 Code (Nightly)" {
		t.Fatalf("parsePSLine = (%d, %q, %v)", ppid, comm, ok)
	}
	if _, _, ok := parsePSLine(""); ok {
		t.Fatal("parsePSLine accepted empty line")
	}
}

func TestAppForPID(t *testing.T) {
	type proc struct {
		ppid int
		comm string
	}
	table := map[int]proc{
		100: {ppid: 1, comm: "/Applications/T3 Code (Nightly).app/Contents/MacOS/T3 Code (Nightly)"},
		200: {ppid: 100, comm: "/opt/homebrew/bin/node"},
		300: {ppid: 1, comm: "/opt/homebrew/bin/node"},
		400: {ppid: 401, comm: "/usr/bin/a"},
		401: {ppid: 400, comm: "/usr/bin/b"},
	}
	inspect := func(pid int) (int, string, error) {
		p, ok := table[pid]
		if !ok {
			return 0, "", errors.New("not running")
		}
		return p.ppid, p.comm, nil
	}

	wantApp := "/Applications/T3 Code (Nightly).app"
	for _, pid := range []int{100, 200} {
		got, err := appForPIDWith(pid, inspect)
		if err != nil || got != wantApp {
			t.Errorf("appForPIDWith(%d) = (%q, %v), want (%q, nil)", pid, got, err, wantApp)
		}
	}
	for _, tc := range []struct {
		name string
		pid  int
	}{
		{"dead", 999},
		{"no app ancestor", 300},
		{"cycle", 400},
		{"zero", 0},
	} {
		if got, err := appForPIDWith(tc.pid, inspect); err == nil {
			t.Errorf("%s (pid %d): got %q, want error", tc.name, tc.pid, got)
		}
	}
	if err := OpenForPID(context.Background(), 0, ""); err == nil {
		t.Error("OpenForPID accepted pid 0")
	}
}
