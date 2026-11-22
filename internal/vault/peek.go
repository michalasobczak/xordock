package vault

import (
	"errors"
	"time"
)

// Summary to stan sejfu do wyświetlenia w aplikacji (S007 U-01).
type Summary struct {
	Seq        uint64
	Snapshots  int
	LastBackup *PointRef
	BackupSnap *Snapshot
	Changes    int // ścieżki zmienione od migawki ostatniego backupu
}

// ErrBusy: superbloki A i B wskazują różne commity (commit w toku albo
// przerwany). Peek niczego nie naprawia; zrobi to Open.
var ErrBusy = errors.New("sejf w trakcie zapisu; stan chwilowo niedostępny")

// Peek czyta stan sejfu tylko do odczytu: bez blokady, bez odzyskiwania po
// awarii i bez GC. Działa także, gdy sejf jest zamontowany przez inny proces
// (S007 §5). Wszystkie obiekty są niezmienne; jeśli GC montującego usunie
// czytany indeks, Peek zwraca błąd, nigdy nieprawdziwy stan.
func Peek(aRoot, bRoot string) (Summary, error) {
	var s Summary
	hA, err := readHeader(aRoot, SideA)
	if err != nil {
		return s, err
	}
	hB, err := readHeader(bRoot, SideB)
	if err != nil {
		return s, err
	}
	if hB.Role == roleBackup {
		return s, ErrSpareCard
	}
	if hA.Vault != hB.Vault {
		return s, ErrWrongCard
	}
	id, err := ParseID(hA.Vault)
	if err != nil {
		return s, err
	}
	sA, err := readSuper(aRoot, SideA, id)
	if err != nil {
		return s, err
	}
	sB, err := readSuper(bRoot, SideB, id)
	if err != nil {
		return s, err
	}
	if sA.Cur != sB.Cur {
		return s, ErrBusy
	}
	ix, err := loadIndexFrom(aRoot, bRoot, id, sA.Cur.Index)
	if err != nil {
		return s, err
	}
	s.Seq = sA.Cur.Seq
	s.Snapshots = len(ix.Snaps)
	best, bestSnap, found := lastReady(ix.Snaps)
	if !found {
		return s, nil
	}
	s.LastBackup, s.BackupSnap = &best, &bestSnap
	old, err := loadIndexFrom(aRoot, bRoot, id, bestSnap.Index)
	if err != nil {
		return s, err
	}
	s.Changes = len(diffIndex(old, ix))
	return s, nil
}

// BackupDue: brak punktu READY albo ostatni starszy niż tydzień (S004 B-01).
func (s Summary) BackupDue(now time.Time) bool {
	return s.LastBackup == nil || now.Sub(s.LastBackup.Time) > 7*24*time.Hour
}
