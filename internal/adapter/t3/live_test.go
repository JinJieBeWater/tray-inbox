package t3

import (
	"bufio"
	"bytes"
	"testing"
	"time"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

// live-stream.jsonl is a real subscribeShell + getConfig session, one frame per line, its text replaced by placeholders.
func TestLiveStreamMapsEveryThread(t *testing.T) {
	sc := bufio.NewScanner(bytes.NewReader(readTestdata(t, "live-stream.jsonl")))
	sc.Buffer(nil, 1<<24)
	var tr *tracker
	var items []item
	for sc.Scan() {
		ms, err := decodeFrame(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			switch {
			case m.Tag == "Exit" && string(m.RequestID) == configRequest:
				env, kinds, err := decodeConfig(m.Exit)
				if err != nil {
					t.Fatal(err)
				}
				tr = newTracker(env, kinds)
			case m.Tag == "Chunk":
				for _, v := range m.Values {
					it, err := decodeItem(v)
					if err != nil {
						t.Fatal(err)
					}
					items = append(items, it)
				}
			}
		}
	}
	if tr == nil {
		t.Fatal("no getConfig answer in the recording")
	}
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	var snap *agent.Snapshot
	for _, it := range items {
		for _, ev := range tr.apply(it, now) {
			if s, ok := ev.(agent.Snapshot); ok {
				if snap != nil {
					t.Fatalf("second snapshot event from %s (patch=%v)", it.Kind, it.Patch)
				}
				snap = &s
			}
		}
	}
	if snap == nil || len(snap.Agents) < 20 {
		t.Fatalf("snapshot = %v, want the recorded threads", snap)
	}
	shown := map[string]bool{}
	for _, a := range snap.Agents {
		shown[a.ID.Native] = true
	}
	var parents, projects int
	for _, a := range snap.Agents {
		if a.Parent != "" && !shown[a.Parent] {
			t.Errorf("%s: parent %s is not shown, so T3 lists it nowhere", a.ID.Native, a.Parent)
		}
		if a.Title == "" || a.Kind == "" || a.Where == "" {
			t.Errorf("%s: title %q kind %q where %q, want all set", a.ID.Native, a.Title, a.Kind, a.Where)
		}
		if a.Parent != "" {
			parents++
		}
		if a.Project != "" {
			projects++
		}
	}
	if parents == 0 || projects == 0 {
		t.Errorf("%d agents with a parent, %d with a project; want some of each", parents, projects)
	}
}
