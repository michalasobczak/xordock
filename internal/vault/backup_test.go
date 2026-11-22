package vault

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func spareRoot(t *testing.T, a string) string {
	return filepath.Join(filepath.Dir(a), "SPARE")
}

func mustGet(t *testing.T, r FileReader, p string) []byte {
	t.Helper()
	got, err := r.GetFile(p)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return got
}

// CG-06, CG-07: a snapshot pins state G1; writing G2 does not change the G1 read,
// and GC does not delete the snapshot's objects. A snapshot does not copy data.
func TestSnapshotKeepsOldVersion(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	g1 := content(2*BlockSize+3, "version-one")
	if err := v.PutFile("/doc", g1); err != nil {
		t.Fatal(err)
	}
	before := countObjects(t, a)
	g1e, _ := v.Stat("/doc")
	s, err := v.CreateSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	// snapshot = only a new index, no block copies
	if got := countObjects(t, a); got > before+1 {
		t.Fatalf("snapshot copied data: %d → %d objects", before, got)
	}
	for i := 0; i < 3; i++ {
		if err := v.PutFile("/doc", content(BlockSize, "version-two")); err != nil {
			t.Fatal(err)
		}
	}
	v.Close()
	v2 := mustOpen(t, a, b) // also gcAll on open
	defer v2.Close()
	w, err := v2.SnapshotView(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustGet(t, w, "/doc"), g1) {
		t.Fatal("snapshot G1 returns different bytes")
	}
	if !bytes.Equal(mustGet(t, v2, "/doc"), content(BlockSize, "version-two")) {
		t.Fatal("current state wrong")
	}
	if ch, _ := v2.Changes(s.ID); len(ch) != 1 || ch[0] != "/doc" {
		t.Fatalf("changes since the snapshot: %v", ch)
	}
	if r := v2.Fsck(); !r.OK() || r.Snapshots != 1 {
		t.Fatalf("fsck: %+v", r)
	}
	// deleting the snapshot releases its objects
	if err := v2.DeleteSnapshot(s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := v2.SnapshotView(s.ID); !errors.Is(err, ErrNoSnapshot) {
		t.Fatal("snapshot still exists")
	}
	// The previous state (fallback point) still points to the snapshot; after another
	// commit its blocks must disappear from both sides.
	v2.Mkdir("/x")
	for _, h := range g1e.Blocks {
		oid, _ := ParseID(h)
		for _, root := range []string{a, b} {
			if _, err := os.Stat(objPath(root, oid)); err == nil {
				t.Fatalf("GC did not release a snapshot block in %s", root)
			}
		}
	}
}

func countObjects(t *testing.T, root string) int {
	n := 0
	filepath.WalkDir(filepath.Join(root, objDir), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// S-04: when a snapshot index cannot be read, GC deletes nothing.
func TestGCStopsWhenSnapshotUnreadable(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/f", content(100, "x"))
	s, _ := v.CreateSnapshot()
	v.PutFile("/f", content(100, "y"))
	v.Close()
	sid, _ := ParseID(s.Index)
	os.Remove(objPath(b, sid)) // damaged snapshot index
	before := countObjects(t, a)
	v2 := mustOpen(t, a, b)
	defer v2.Close()
	v2.PutFile("/f", content(100, "z"))
	if got := countObjects(t, a); got < before {
		t.Fatalf("GC deleted despite uncertainty: %d → %d", before, got)
	}
	if r := v2.Fsck(); r.OK() {
		t.Fatal("fsck did not report the damaged snapshot")
	}
}

// CG-08, SC-04: losing the working card → restore of the exact G1 bytes
// from the A snapshot and the spare card; later changes do not come back.
func TestRecoverAfterLosingWorkingCard(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	g1 := content(3*BlockSize+11, "data-g1")
	v.Mkdir("/dir")
	v.PutFile("/dir/file", g1)
	s, _ := v.CreateSnapshot()
	p, err := v.Backup(spare, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Snapshot != s.ID || p.State != stateReady {
		t.Fatalf("point: %+v", p)
	}
	if ref, _, ok := v.LastBackup(); !ok || !ref.Ready || ref.Point != p.ID {
		t.Fatalf("backup status: %+v %v", ref, ok)
	}
	v.PutFile("/dir/file", content(10, "after-backup")) // G2, not on the spare card
	v.PutFile("/new", content(10, "after-backup"))
	v.Close()
	if err := os.RemoveAll(b); err != nil { // lost working card
		t.Fatal(err)
	}
	if _, err := Open(a, b, Options{}); err == nil {
		t.Fatal("vault opened without the working card")
	}
	if _, err := Open(a, spare, Options{}); !errors.Is(err, ErrSpareCard) {
		t.Fatalf("spare card used as the working card: %v", err) // CG-09
	}
	r, err := OpenRecovery(a, spare)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	pts, bad := r.Points()
	if len(pts) != 1 || len(bad) != 0 {
		t.Fatalf("points: %v %v", pts, bad)
	}
	if _, n, err := r.Verify(""); err != nil || n != 1 {
		t.Fatalf("verify: %d %v", n, err)
	}
	w, _, err := r.View("")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(mustGet(t, w, "/dir/file"), g1) {
		t.Fatal("recovered bytes differ from G1")
	}
	if _, ok := w.Stat("/new"); ok {
		t.Fatal("recovery point contains a change not saved before it")
	}
	// New vault with new masks (R-04).
	d := t.TempDir()
	nv, err := Create(filepath.Join(d, "A2"), filepath.Join(d, "B2"), "recovered", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := CopyInto(w, nv); err != nil || n != 1 {
		t.Fatalf("copying: %d %v", n, err)
	}
	if !bytes.Equal(mustGet(t, nv, "/dir/file"), g1) {
		t.Fatal("new vault has different bytes")
	}
	// Plain export (rollback).
	out := filepath.Join(d, "export")
	if n, err := ExportTo(w, out); err != nil || n != 1 {
		t.Fatalf("export: %d %v", n, err)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "dir", "file")); !bytes.Equal(got, g1) {
		t.Fatal("export has different bytes")
	}
}

// CG-09: the B_G1 spare card does not join G2 objects or anything outside the point.
func TestSpareNotUsedForNewerObjects(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	v.PutFile("/f", content(100, "g1"))
	v.CreateSnapshot()
	if _, err := v.Backup(spare, ""); err != nil {
		t.Fatal(err)
	}
	v.PutFile("/f", content(100, "g2"))
	e, _ := v.Stat("/f")
	v.Close()
	r, err := OpenRecovery(a, spare)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	w, _, _ := r.View("")
	oid, _ := ParseID(e.Blocks[0])
	if _, err := w.read(oid); err == nil {
		t.Fatal("point G1 joined an object from G2")
	}
	if !bytes.Equal(mustGet(t, w, "/f"), content(100, "g1")) {
		t.Fatal("G1 wrong")
	}
}

// CG-10: interrupting the backup at any stage leaves the previous READY point
// and does not create a READY point from the interrupted attempt.
func TestInterruptedBackupKeepsPreviousPoint(t *testing.T) {
	for _, stage := range []string{"backup-copy", "backup-verify", "backup-ready"} {
		t.Run(stage, func(t *testing.T) {
			a, b := roots(t)
			spare := spareRoot(t, a)
			fail := ""
			v := mustCreate(t, a, b, Options{Hook: func(s string) error {
				if s == fail {
					return errors.New("awaria: " + s)
				}
				return nil
			}})
			defer v.Close()
			v.PutFile("/f", content(2*BlockSize, "g1"))
			v.CreateSnapshot()
			p1, err := v.Backup(spare, "")
			if err != nil {
				t.Fatal(err)
			}
			v.PutFile("/f", content(2*BlockSize, "g2"))
			v.CreateSnapshot()
			fail = stage
			if _, err := v.Backup(spare, ""); err == nil {
				t.Fatal("backup did not report the failure")
			}
			fail = ""
			pts, _ := ReadPoints(spare, v.ID())
			if len(pts) != 1 || pts[0].ID != p1.ID {
				t.Fatalf("points after the failure: %+v", pts)
			}
			ix, err := loadIndexFrom(a, spare, v.ID(), p1.Index)
			if err != nil {
				t.Fatal(err)
			}
			w := &View{ix: ix, read: func(id ID) ([]byte, error) { return readPair(a, spare, v.ID(), id) }}
			if !bytes.Equal(mustGet(t, w, "/f"), content(2*BlockSize, "g1")) {
				t.Fatal("previous point damaged")
			}
			// After the failure the next backup succeeds.
			if _, err := v.Backup(spare, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// CG-11: no space → error, nothing copied, previous point intact.
func TestBackupNoSpace(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(100, "g1"))
	v.CreateSnapshot()
	p1, err := v.Backup(spare, "")
	if err != nil {
		t.Fatal(err)
	}
	v.PutFile("/g", content(3*BlockSize, "g2"))
	v.CreateSnapshot()
	before := countObjects(t, spare)
	old := FreeSpace
	FreeSpace = func(string) (uint64, error) { return 1 << 20, nil }
	defer func() { FreeSpace = old }()
	if _, err := v.Backup(spare, ""); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("expected ErrNoSpace, got %v", err)
	}
	if countObjects(t, spare) != before {
		t.Fatal("backup copied despite lack of space")
	}
	if pts, _ := ReadPoints(spare, v.ID()); len(pts) != 1 || pts[0].ID != p1.ID {
		t.Fatal("previous point changed")
	}
}

// B-04, S-04: a snapshot with a point is pinned; prune keeps the newest
// point and removes the pins of older ones.
func TestPruneKeepsNewestAndUnpins(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(BlockSize, "g1"))
	s1, _ := v.CreateSnapshot()
	v.Backup(spare, "")
	if err := v.DeleteSnapshot(s1.ID); !errors.Is(err, ErrPinned) {
		t.Fatalf("pinned snapshot deleted: %v", err)
	}
	v.PutFile("/f", content(BlockSize, "g2"))
	v.CreateSnapshot()
	p2, err := v.Backup(spare, "")
	if err != nil {
		t.Fatal(err)
	}
	np, no, err := v.PruneBackup(spare)
	if err != nil || np != 1 || no == 0 {
		t.Fatalf("prune: %d %d %v", np, no, err)
	}
	if pts, _ := ReadPoints(spare, v.ID()); len(pts) != 1 || pts[0].ID != p2.ID {
		t.Fatal("prune did not keep the newest point")
	}
	if err := v.DeleteSnapshot(s1.ID); err != nil {
		t.Fatalf("snapshot still pinned after prune: %v", err)
	}
	v.Close()
	r, err := OpenRecovery(a, spare)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, _, err := r.Verify(""); err != nil {
		t.Fatalf("newest point after prune: %v", err)
	}
}

// CG-12: the spare card and point records contain no content or names.
func TestSpareHoldsNoPlaintext(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	marker := []byte("SECRET-MARKER-BACKUP")
	name := "SECRET-NAME-BACKUP"
	v.PutFile("/"+name, content(2*BlockSize, string(marker)))
	v.CreateSnapshot()
	if _, err := v.Backup(spare, ""); err != nil {
		t.Fatal(err)
	}
	scanFor(t, spare, marker, []byte(name))
	scanFor(t, a, marker, []byte(name))
}

// Backup refuses the working card given as a spare card.
func TestBackupRejectsWorkingCard(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(10, "x"))
	v.CreateSnapshot()
	if _, err := v.Backup(b, ""); err == nil {
		t.Fatal("working card accepted as a spare card")
	}
}

// S005 / R4-01: prune does not delete older points when the newest one is
// incomplete (B-04: we never lose the only good copy).
func TestPruneRefusesIncompleteNewest(t *testing.T) {
	a, b := roots(t)
	spare := spareRoot(t, a)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	v.PutFile("/f", content(BlockSize, "g1"))
	v.CreateSnapshot()
	p1, _ := v.Backup(spare, "")
	v.PutFile("/f", content(BlockSize, "g2"))
	v.CreateSnapshot()
	v.Backup(spare, "")
	e, _ := v.Stat("/f")
	oid, _ := ParseID(e.Blocks[0])
	os.WriteFile(objPath(spare, oid), []byte("damaged"), 0o600) // block of the newest point
	if _, _, err := v.PruneBackup(spare); err == nil {
		t.Fatal("prune ran despite an incomplete newest point")
	}
	pts, _ := ReadPoints(spare, v.ID())
	if len(pts) != 2 || pts[1].ID != p1.ID {
		t.Fatalf("prune deleted points: %+v", pts)
	}
}

// S005 / R4-02: an index with a path leaving the tree is rejected,
// and export does not write outside the target directory.
func TestIndexPathsValidated(t *testing.T) {
	for _, bad := range []string{"/../escape", "/a/../../b", "relative", "/a//b", "/a/"} {
		ix := newIndex()
		ix.Entries[bad] = &Entry{Ino: 5}
		if err := ix.validate(); err == nil {
			t.Fatalf("index with path %q accepted", bad)
		}
	}
	ix := newIndex()
	ix.Entries["/k"] = &Entry{Ino: 2, Dir: true}
	ix.Entries["/k/p"] = &Entry{Ino: 3}
	if err := ix.validate(); err != nil {
		t.Fatal(err)
	}
	ix.Entries["/no/parent"] = &Entry{Ino: 4}
	if err := ix.validate(); err == nil {
		t.Fatal("entry without a parent directory accepted")
	}
}
