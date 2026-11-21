package t3

import "testing"

func TestDecodeItem(t *testing.T) {
	snap := decode(t, readTestdata(t, "shell-snapshot.json"))
	if snap.Kind != "snapshot" || snap.Patch || snap.Snapshot == nil || len(snap.Snapshot.Threads) != 13 || len(snap.Snapshot.Projects) != 2 || snap.Snapshot.SnapshotSequence != 120 {
		t.Fatalf("snapshot decoded as %+v", snap)
	}
	if patch := decode(t, readTestdata(t, "shell-patch.json")); patch.Kind != "snapshot" || !patch.Patch {
		t.Fatalf("patch decoded as %+v", patch)
	}
	if it := decode(t, []byte(`{"kind":"snapshot-required"}`)); it.Kind != "snapshot-required" {
		t.Fatalf("unknown kind decoded as %+v", it)
	}
}

func TestDecodeItemRejectsMalformed(t *testing.T) {
	for _, b := range []string{
		`not json`,
		`{"kind":"snapshot"}`,
		`{"kind":"thread.updated","sequence":3,"location":"active"}`,
		`{"kind":"project.updated","sequence":3}`,
	} {
		if _, err := decodeItem([]byte(b)); err == nil {
			t.Errorf("decodeItem(%s) succeeded", b)
		}
	}
}
