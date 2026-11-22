package vault

import (
	"errors"
	"time"
)

// Summary is the vault state shown in the app (S007 U-01).
type Summary struct {
	Seq        uint64
	Snapshots  int
	LastBackup *PointRef
	BackupSnap *Snapshot
	Changes    int // paths changed since the last backup's snapshot
}

// ErrBusy: superblocks A and B point to different commits (commit in progress or
// interrupted). Peek repairs nothing; Open will do that.
var ErrBusy = errors.New("vault is being written; state temporarily unavailable")

// Peek reads the vault state read-only: no lock, no crash recovery
// and no GC. It works even when the vault is mounted by another process
// (S007 §5). All objects are immutable; if the mounting process's GC deletes
// the index being read, Peek returns an error, never a false state.
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

// BackupDue: no READY point or the last one is older than a week (S004 B-01).
func (s Summary) BackupDue(now time.Time) bool {
	return s.LastBackup == nil || now.Sub(s.LastBackup.Time) > 7*24*time.Hour
}
