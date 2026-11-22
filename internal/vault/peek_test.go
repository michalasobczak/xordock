package vault

import (
	"testing"
	"time"
)

// S007 G7: Peek czyta stan przy otwartym (zamontowanym) sejfie, czyli przy
// blokadzie trzymanej przez inny uchwyt, i niczego nie zmienia.
func TestPeekWhileOpen(t *testing.T) {
	a, b := roots(t)
	v := mustCreate(t, a, b, Options{})
	defer v.Close()
	s, err := Peek(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if s.LastBackup != nil || !s.BackupDue(time.Now()) {
		t.Fatal("nowy sejf bez backupu musi wymagać backupu")
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
		t.Fatal("reguła 7 dni")
	}
	if countObjects(t, a) != before {
		t.Fatal("Peek zmienił udziały")
	}
	if _, err := Peek(a, spareRoot(t, a)); err != ErrSpareCard {
		t.Fatalf("Peek z kartą zapasową jako B: %v", err)
	}
}
