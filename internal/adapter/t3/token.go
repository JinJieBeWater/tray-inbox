package t3

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

// TokenStore keeps the bearer token across restarts. Get returns an error, such as ErrNoToken, when it has none.
type TokenStore interface {
	Get() (string, error)
	Set(string) error
}

var ErrNoToken = errors.New("t3: no token")

var errRejected = errors.New("t3: credentials rejected")

func (r *runner) findT3(ctx context.Context) string {
	if r.cfg.T3 != "" {
		return r.cfg.T3
	}
	if r.cfg.UserData != "" {
		bundled := filepath.Join(filepath.Dir(r.cfg.UserData), "bin", "t3")
		if fi, err := os.Stat(bundled); err == nil && !fi.IsDir() && fi.Mode().Perm()&0o111 != 0 {
			return bundled
		}
	}
	if !r.lookedT3 && r.cfg.LookT3 != nil {
		r.lookedT3 = true
		return r.cfg.LookT3(ctx)
	}
	return ""
}

// issue signs a read-only session. It writes T3's database, so it runs only when there is no token or the
// server rejected one that it had accepted before.
func (r *runner) issue(ctx context.Context) (string, agent.Reason) {
	bin := r.findT3(ctx)
	if bin == "" {
		return "", agent.ReasonNoCommand
	}
	ctx, cancel := context.WithTimeout(ctx, issueTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "auth", "session", "issue",
		"--scope", "orchestration:read", "--ttl", "30d", "--label", "trayinbox", "--json").Output()
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return "", agent.ReasonNoCommand
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err != nil || json.Unmarshal(out, &issued) != nil || issued.Token == "" {
		return "", agent.ReasonReconnecting
	}
	if err := r.cfg.Tokens.Set(issued.Token); err != nil {
		log.Print("t3: the issued token is kept only until TrayInbox quits: ", err)
	}
	r.token, r.unproven = issued.Token, true
	return issued.Token, ""
}
