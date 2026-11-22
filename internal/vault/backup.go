package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Backup to a spare card (S004 §5.5). The spare card stores copies of the
// B shares of one snapshot's objects and a READY recovery point record.
// It is not a separate key: it is the same share B in the same version (B-02).
//
// Order (B-03, CG-10):
//  1. pin the point in the vault index (the snapshot will not disappear meanwhile),
//  2. copy the missing B shares to obj/ (tmp + rename),
//  3. sync and re-verify all required shares,
//  4. write the points/<id> record with a full fsync: only now is the point READY,
//  5. mark the pin as Ready in the index (information for status).
//
// An interruption at any point leaves the previous points untouched:
// copying only adds files, it never deletes or overwrites them.

const (
	pointsDir   = "points"
	stateReady  = "READY"
	spaceMargin = 1 << 20
)

var (
	ErrNoSpace  = errors.New("not enough space on the spare card; the previous recovery point stays unchanged")
	ErrNoPoint  = errors.New("no READY recovery point")
	ErrSameCard = errors.New("the spare card must be a different device than the SSD and the working card")
)

// FreeSpace returns free space in bytes (replaced in tests, CG-11).
var FreeSpace = func(p string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// Point is a recovery point record on the spare card. It contains only
// IDs, numbers and times (D-04), no file names.
type Point struct {
	Format   int       `json:"format"`
	Vault    string    `json:"vault"`
	Media    string    `json:"media"`
	ID       string    `json:"id"`
	Snapshot string    `json:"snapshot"`
	Seq      uint64    `json:"seq"`
	Index    string    `json:"index"`
	Created  time.Time `json:"created"`
	Objects  int       `json:"objects"`
	Bytes    int64     `json:"bytes"`
	State    string    `json:"state"`
}

// prepareSpare creates or checks the spare card header and returns
// the medium ID.
func (v *Vault) prepareSpare(spare string) (string, error) { return v.prepareCopy(spare, SideB) }

// ErrACopyOnCard: the share A copy cannot be on a card (S008 G4).
var ErrACopyOnCard = errors.New("the share A copy cannot be on the working card or the spare card")

// within: p is inside the root directory or is it.
func within(p, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// prepareCopy creates or checks the header of the copy of side side (B: spare
// card, S004; A: copy in iCloud Drive, S008) and returns the medium ID.
func (v *Vault) prepareCopy(dst string, side Side) (string, error) {
	for _, r := range []string{v.aRoot, v.bRoot} {
		if within(dst, r) || within(r, dst) {
			if side == SideA {
				return "", ErrACopyOnCard
			}
			return "", ErrSameCard
		}
	}
	if SameDeviceCheck {
		if err := os.MkdirAll(dst, 0o700); err != nil {
			return "", err
		}
		d := devOf(dst)
		switch {
		case side == SideB && (d == devOf(v.aRoot) || d == devOf(v.bRoot)):
			return "", ErrSameCard
		case side == SideA && d == devOf(v.bRoot):
			return "", ErrACopyOnCard
		}
	}
	h, err := readHeader(dst, side)
	switch {
	case err == nil:
		if h.Vault != v.id.String() {
			return "", ErrWrongCard
		}
		if h.Role != roleBackup || h.Media == "" {
			if side == SideA {
				return "", errors.New("the given directory is a live share A, not a copy")
			}
			return "", errors.New("the given directory is the working card, not a spare card")
		}
		return h.Media, nil
	case errors.Is(err, fs.ErrNotExist):
		mid, err := newID()
		if err != nil {
			return "", err
		}
		h := header{Format: FormatVersion, Vault: v.id.String(), Side: string(side), Name: v.name,
			BlockSize: BlockSize, Created: time.Now().UTC(), Masks: v.MaskKind(), Role: roleBackup, Media: mid.String()}
		data, _ := json.MarshalIndent(h, "", "  ")
		if err := writeAtomic(filepath.Join(dst, headerFile), data, true); err != nil {
			return "", err
		}
		return h.Media, nil
	default:
		return "", fmt.Errorf("copy header %c: %w", side, err)
	}
}

// Backup copies the B shares of snapshot snapID (empty: the newest) to the spare card
// and writes a READY recovery point. Requires the working and spare cards
// connected at the same time.
func (v *Vault) Backup(spare, snapID string) (Point, error) {
	ps, err := v.backup(snapID, []copyTarget{{root: spare, side: SideB}})
	if err != nil {
		return Point{}, err
	}
	return ps[0], nil
}

// BackupPair writes one recovery point in two places: B shares
// to the spare card and A shares of the same snapshot to the A copy (S008). When
// the A copy fails, the B point stays READY and the error says so.
func (v *Vault) BackupPair(spare, aCopy, snapID string) (Point, Point, error) {
	if within(aCopy, spare) || within(spare, aCopy) {
		return Point{}, Point{}, ErrACopyOnCard
	}
	if SameDeviceCheck {
		for _, d := range []string{spare, aCopy} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				return Point{}, Point{}, err
			}
		}
		if devOf(aCopy) == devOf(spare) {
			return Point{}, Point{}, ErrACopyOnCard
		}
	}
	ps, err := v.backup(snapID, []copyTarget{{root: spare, side: SideB}, {root: aCopy, side: SideA}})
	var pb, pa Point
	if len(ps) > 0 {
		pb = ps[0]
	}
	if len(ps) > 1 {
		pa = ps[1]
	}
	if err != nil && len(ps) == 1 {
		err = fmt.Errorf("point %s is READY on the spare card, but the share A copy was not created: %w", pb.ID, err)
	}
	return pb, pa, err
}

type copyTarget struct {
	root    string
	side    Side
	media   string
	missing []ID
	ref     PointRef
}

// stage: stage names for test hooks (B: as in S004, A: acopy-*).
func (t copyTarget) stage(s string) string {
	if t.side == SideA {
		return "acopy-" + s
	}
	return "backup-" + s
}

// src: the live share of the copy's side; other: the live share of the opposite side.
func (v *Vault) sideRoots(side Side) (src, other string, otherSide Side) {
	if side == SideA {
		return v.aRoot, v.bRoot, SideB
	}
	return v.bRoot, v.aRoot, SideA
}

func (v *Vault) backup(snapID string, targets []copyTarget) ([]Point, error) {
	v.mu.Lock()
	if v.broken != nil {
		v.mu.Unlock()
		return nil, ErrBroken
	}
	snaps := v.idx.Snaps
	v.mu.Unlock()
	if len(snaps) == 0 {
		return nil, errors.New("no snapshots; first run: xordock snapshot create")
	}
	snap := snaps[len(snaps)-1]
	if snapID != "" {
		found := false
		for _, s := range snaps {
			if s.ID == snapID {
				snap, found = s, true
			}
		}
		if !found {
			return nil, ErrNoSnapshot
		}
	}
	for i := range targets {
		media, err := v.prepareCopy(targets[i].root, targets[i].side)
		if err != nil {
			return nil, err
		}
		targets[i].media = media
	}

	// List of required objects and space (B-04: refuse before any change).
	ix, err := v.loadIndex(snap.Index)
	if err != nil {
		return nil, err
	}
	need := map[string]bool{snap.Index: true}
	ix.refs(need)
	ids := make([]ID, 0, len(need))
	for h := range need {
		id, err := ParseID(h)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	sizes := map[ID]int64{}
	var total int64
	for i := range targets {
		t := &targets[i]
		src, _, _ := v.sideRoots(t.side)
		var missingBytes int64
		for _, id := range ids {
			st, err := os.Stat(objPath(src, id))
			if err != nil {
				return nil, fmt.Errorf("share %c %s: %w", t.side, id, err)
			}
			if t.side == SideB {
				sizes[id] = st.Size()
			}
			if _, err := os.Stat(objPath(t.root, id)); err != nil {
				t.missing = append(t.missing, id)
				missingBytes += st.Size()
			}
		}
		free, err := FreeSpace(t.root)
		if err != nil {
			return nil, err
		}
		if uint64(missingBytes+spaceMargin) > free {
			where := "on the spare card"
			if t.side == SideA {
				where = "at the share A copy target"
			}
			return nil, fmt.Errorf("%w (%s need %d B, free %d B)", ErrNoSpace, where, missingBytes+spaceMargin, free)
		}
	}
	for _, n := range sizes {
		total += n
	}

	pid, err := newID()
	if err != nil {
		return nil, err
	}
	for i := range targets {
		targets[i].ref = PointRef{Media: targets[i].media, Point: pid.String()}
	}
	if err := v.Commit(func(ix *Index) error {
		for _, t := range targets {
			if err := pinPoint(ix, snap.ID, t.ref); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("pinning the snapshot: %w", err)
	}

	var out []Point
	for _, t := range targets {
		p, err := v.copySide(t, snap, pid, ids, total)
		if err != nil {
			return out, err
		}
		out = append(out, p)
	}
	return out, nil
}

// copySide copies the missing shares of side t.side of the snapshot, verifies the target
// and the opposite side, writes the READY point record and marks the pin.
// Copying without the vault lock: snapshot objects are immutable and pinned.
func (v *Vault) copySide(t copyTarget, snap Snapshot, pid ID, ids []ID, total int64) (Point, error) {
	src, other, otherSide := v.sideRoots(t.side)
	dirs := map[string]bool{}
	for _, id := range t.missing {
		if err := v.at(t.stage("copy")); err != nil {
			return Point{}, err
		}
		raw, err := os.ReadFile(objPath(src, id))
		if err != nil {
			return Point{}, fmt.Errorf("share %c %s: %w", t.side, id, err)
		}
		// Only a valid share of this side goes into the copy (S008 G3).
		if _, err := decodeShare(raw, t.side, v.id, id); err != nil {
			return Point{}, err
		}
		dst := objPath(t.root, id)
		if err := writeAtomic(dst, raw, false); err != nil {
			return Point{}, fmt.Errorf("writing copy %c: %w", t.side, err)
		}
		dirs[filepath.Dir(dst)] = true
	}
	for d := range dirs {
		syncDir(d, false)
	}
	if err := barrier(t.root); err != nil {
		return Point{}, err
	}
	if err := v.at(t.stage("verify")); err != nil {
		return Point{}, err
	}
	for _, id := range ids {
		raw, err := os.ReadFile(objPath(t.root, id))
		if err == nil {
			_, err = decodeShare(raw, t.side, v.id, id)
		}
		if err != nil {
			return Point{}, fmt.Errorf("verifying copy %c %s: %w", t.side, id, err)
		}
		rawO, err := os.ReadFile(objPath(other, id))
		if err == nil {
			_, err = decodeShare(rawO, otherSide, v.id, id)
		}
		if err != nil {
			return Point{}, fmt.Errorf("share %c of snapshot %s: %w", otherSide, id, err)
		}
	}
	if err := v.at(t.stage("ready")); err != nil {
		return Point{}, err
	}
	p := Point{Format: FormatVersion, Vault: v.id.String(), Media: t.media, ID: pid.String(), Snapshot: snap.ID,
		Seq: snap.Seq, Index: snap.Index, Created: time.Now().UTC(), Objects: len(ids), Bytes: total, State: stateReady}
	if err := writeAtomic(filepath.Join(t.root, pointsDir, p.ID), seal(p), true); err != nil {
		return Point{}, fmt.Errorf("writing the recovery point: %w", err)
	}
	ref := t.ref
	ref.Ready, ref.Time = true, p.Created
	if err := v.Commit(func(ix *Index) error { return pinPoint(ix, snap.ID, ref) }); err != nil {
		// The point is READY on the medium; a missing flag in the index affects only status.
		Logf("xordock: point %s is READY, but it was not recorded in the index: %v", p.ID, err)
	}
	return p, nil
}

// pinPoint adds or updates the point's pin in the snapshot.
func pinPoint(ix *Index, snapID string, ref PointRef) error {
	i, ok := ix.snap(snapID)
	if !ok {
		return ErrNoSnapshot
	}
	for j, r := range ix.Snaps[i].Points {
		if r.Media == ref.Media && r.Point == ref.Point {
			ix.Snaps[i].Points[j] = ref
			return nil
		}
	}
	ix.Snaps[i].Points = append(ix.Snaps[i].Points, ref)
	return nil
}

// ReadPoints returns valid READY points from the spare card (newest
// first) and descriptions of rejected records.
func ReadPoints(spare string, vault ID) ([]Point, []string) {
	var out []Point
	var bad []string
	entries, _ := os.ReadDir(filepath.Join(spare, pointsDir))
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(spare, pointsDir, e.Name()))
		var p Point
		if err == nil {
			err = unseal(data, &p)
		}
		switch {
		case err != nil:
			bad = append(bad, fmt.Sprintf("point %s: %v", e.Name(), err))
		case p.Vault != vault.String() || p.ID != e.Name():
			bad = append(bad, fmt.Sprintf("point %s: belongs to a different vault or has a bad name", e.Name()))
		case p.State != stateReady:
			bad = append(bad, fmt.Sprintf("point %s: state %q", e.Name(), p.State))
		default:
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seq != out[j].Seq {
			return out[i].Seq > out[j].Seq
		}
		return out[i].Created.After(out[j].Created)
	})
	return out, bad
}

// PruneBackup keeps only the newest READY point on the spare card,
// deletes older points and shares no longer needed, and then removes their
// pins in the vault. This is an explicit retention decision by the user (B-04);
// it never deletes the last good point.
func (v *Vault) PruneBackup(spare string) (removedPoints, removedObjects int, err error) {
	return v.pruneCopy(spare, SideB)
}

// PruneACopy: the same for the share A copy (S008 K-03).
func (v *Vault) PruneACopy(aCopy string) (removedPoints, removedObjects int, err error) {
	return v.pruneCopy(aCopy, SideA)
}

func (v *Vault) pruneCopy(dst string, side Side) (removedPoints, removedObjects int, err error) {
	media, err := v.prepareCopy(dst, side)
	if err != nil {
		return 0, 0, err
	}
	points, _ := ReadPoints(dst, v.id)
	if len(points) == 0 {
		return 0, 0, ErrNoPoint
	}
	keep := points[0]
	_, other, otherSide := v.sideRoots(side)
	aSrc, bSrc := other, dst
	if side == SideA {
		aSrc, bSrc = dst, other
	}
	ix, err := loadIndexFrom(aSrc, bSrc, v.id, keep.Index)
	if err != nil {
		return 0, 0, fmt.Errorf("the newest point %s cannot be read, deleting nothing: %w", keep.ID, err)
	}
	live := map[string]bool{keep.Index: true}
	ix.refs(live)
	// The newest point must be complete on both sides before we delete
	// anything older (B-04, S005 R4-01).
	for h := range live {
		id, err := ParseID(h)
		if err != nil {
			return 0, 0, err
		}
		for _, s := range []struct {
			root string
			side Side
		}{{dst, side}, {other, otherSide}} {
			raw, err := os.ReadFile(objPath(s.root, id))
			if err == nil {
				_, err = decodeShare(raw, s.side, v.id, id)
			}
			if err != nil {
				return 0, 0, fmt.Errorf("the newest point %s is incomplete (%c/%s: %v), deleting nothing", keep.ID, s.side, id, err)
			}
		}
	}
	for _, p := range points[1:] {
		if err := os.Remove(filepath.Join(dst, pointsDir, p.ID)); err != nil {
			return removedPoints, 0, err
		}
		removedPoints++
	}
	syncDir(filepath.Join(dst, pointsDir), true)
	filepath.WalkDir(filepath.Join(dst, objDir), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !live[d.Name()] {
			if os.Remove(p) == nil {
				removedObjects++
			}
		}
		return nil
	})
	err = v.Commit(func(ix *Index) error {
		for i := range ix.Snaps {
			var kept []PointRef
			for _, r := range ix.Snaps[i].Points {
				if r.Media != media || r.Point == keep.ID {
					kept = append(kept, r)
				}
			}
			ix.Snaps[i].Points = kept
		}
		return nil
	})
	return removedPoints, removedObjects, err
}

// LastBackup returns the newest confirmed pin (G-08) and its snapshot.
func (v *Vault) LastBackup() (PointRef, Snapshot, bool) {
	return lastReady(v.Snapshots())
}

// lastReady picks the newest READY point among the snapshots.
func lastReady(snaps []Snapshot) (PointRef, Snapshot, bool) {
	var best PointRef
	var bestSnap Snapshot
	found := false
	for _, s := range snaps {
		for _, r := range s.Points {
			if r.Ready && (!found || s.Seq > bestSnap.Seq || (s.Seq == bestSnap.Seq && r.Time.After(best.Time))) {
				best, bestSnap, found = r, s, true
			}
		}
	}
	return best, bestSnap, found
}
