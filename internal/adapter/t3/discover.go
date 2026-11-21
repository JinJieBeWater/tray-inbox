package t3

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// waitRuntime waits for server-runtime.json to change, or for later. It returns false when ctx ends.
func waitRuntime(ctx context.Context, dir *fsnotify.Watcher, later <-chan time.Time) bool {
	var events <-chan fsnotify.Event
	var errs <-chan error
	if dir != nil {
		events, errs = dir.Events, dir.Errors
	}
	for {
		select {
		case <-ctx.Done():
			return false
		case <-later:
			return true
		case ev, ok := <-events:
			if !ok {
				events = nil
			} else if filepath.Base(ev.Name) == runtimeFile {
				return true
			}
		case _, ok := <-errs:
			if !ok {
				errs = nil
			}
		}
	}
}

func (a *Adapter) origin() (string, error) {
	b, err := os.ReadFile(filepath.Join(a.cfg.UserData, runtimeFile))
	if err != nil {
		return "", err
	}
	var rt struct {
		Origin string `json:"origin"`
	}
	if err := json.Unmarshal(b, &rt); err != nil {
		return "", err
	}
	if rt.Origin == "" {
		return "", errors.New("t3: " + runtimeFile + " has no origin")
	}
	return rt.Origin, nil
}
