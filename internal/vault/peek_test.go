package vault

import (
	"testing"
	"time"
)

// S007 G7: Peek reads the state of an open (mounted) vault, i.e. with
// the lock held by another handle, and changes nothing.
func TestPeekWhileOpen(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	s, err := Peek(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if s.LastBackup != nil || !s.BackupDue(time.Now()) {
		t.Fatal("a new vault without a backup must require a backup")
	}
	if err := v.PutFile("/doc", content(BlockSize+5, "x")); err != nil {
		t.Fatal(err)
	}
	snap, err := v.CreateSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Backup(spareRoot(t, a), snap.ID); err != nil {
		t.Fatal(err)
	}
	if err := v.PutFile("/nowy", []byte("y")); err != nil {
		t.Fatal(err)
	}
	before := countObjects(t, a)
	s, err = Peek(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if s.LastBackup == nil || s.BackupSnap.ID != snap.ID || s.Changes != 1 || s.Snapshots != 1 || s.Seq != v.Seq() {
		t.Fatalf("Peek: %+v", s)
	}
	if s.BackupDue(time.Now()) || !s.BackupDue(time.Now().Add(8*24*time.Hour)) {
		t.Fatal("7-day rule")
	}
	if countObjects(t, a) != before {
		t.Fatal("Peek changed the shares")
	}
	if _, err := Peek(a, spareRoot(t, a)); err != ErrSpareCard {
		t.Fatalf("Peek with the spare card as B: %v", err)
	}
}
