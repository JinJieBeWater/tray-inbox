package t3

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os/exec"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

// TokenStore keeps the bearer token across restarts. Get returns an error, such as ErrNoToken, when it has none.
type TokenStore interface {
	Get() (string, error)
	Set(string) error
}

var ErrNoToken = errors.New("t3: no token")

var errRejected = errors.New("t3: credentials rejected")

// issue signs a read-only session. It writes T3's database, so it runs only when there is no token or the
// server rejected one that it had accepted before.
func (r *runner) issue(ctx context.Context) (string, agent.Reason) {
	if r.cfg.T3 == "" {
		return "", agent.ReasonNoCommand
	}
	ctx, cancel := context.WithTimeout(ctx, issueTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, r.cfg.T3, "auth", "session", "issue",
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
