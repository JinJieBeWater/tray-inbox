//go:build darwin

package system

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const activateTimeout = 5 * time.Second

// OpenForPID activates the macOS .app bundle owning pid (or an ancestor of pid),
// optionally delivering targetURL to that exact bundle. It fails if pid is not
// running or has no enclosing .app bundle, so it never launches a stopped app.
func OpenForPID(ctx context.Context, pid int, targetURL string) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	ctx, cancel := context.WithTimeout(ctx, activateTimeout)
	defer cancel()

	app, err := appForPID(ctx, pid)
	if err != nil {
		return err
	}
	args := []string{"-a", app}
	if targetURL != "" {
		args = append(args, targetURL)
	}
	return exec.CommandContext(ctx, "open", args...).Run()
}

func appForPID(ctx context.Context, pid int) (string, error) {
	return appForPIDWith(pid, func(p int) (int, string, error) {
		out, err := exec.CommandContext(ctx, "ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(p)).Output()
		if err != nil {
			return 0, "", fmt.Errorf("process %d is not running: %w", p, err)
		}
		ppid, comm, ok := parsePSLine(string(out))
		if !ok {
			return 0, "", fmt.Errorf("process %d is not running", p)
		}
		return ppid, comm, nil
	})
}

func appForPIDWith(pid int, inspect func(int) (int, string, error)) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("invalid pid %d", pid)
	}
	seen := map[int]bool{}
	cur := pid
	for range 8 {
		if cur <= 1 || seen[cur] {
			break
		}
		seen[cur] = true
		ppid, comm, err := inspect(cur)
		if err != nil {
			return "", err
		}
		if app, ok := appBundle(comm); ok {
			return app, nil
		}
		cur = ppid
	}
	return "", fmt.Errorf("no macOS app bundle for pid %d", pid)
}

func appBundle(execPath string) (string, bool) {
	const marker = ".app/Contents/"
	i := strings.Index(execPath, marker)
	if i < 0 {
		return "", false
	}
	return execPath[:i+len(".app")], true
}

func parsePSLine(line string) (ppid int, comm string, ok bool) {
	s := strings.TrimSpace(line)
	if s == "" {
		return 0, "", false
	}
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return 0, "", false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, "", false
	}
	comm = strings.TrimSpace(s[i:])
	if comm == "" {
		return 0, "", false
	}
	return n, comm, true
}
