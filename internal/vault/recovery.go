package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"xordock/internal/split"
)

// Recovery after losing the working card (S004 §5.6). We join objects
// from share A on the SSD (pinned by a snapshot) and the copy of share B from the spare
// card. Only for a READY point and only the objects of its index: we never
// mix A_G with B_(G+N) (R-02, CG-09). We write nothing to the spare
// card or to share A (R-03); the only file is the vault lock.

type Recovery struct {
	aRoot, spare string
	id           ID
	Media        string
	lock         *os.File
	aCopy, bCopy bool // the side is a copy (backup role) with point records
}

// OpenRecovery opens recovery from share A (live or the copy from iCloud,
// S008) and share B (spare or working card). At least one side
// must be a copy with point records. Copies are not locked or changed.
func OpenRecovery(aRoot, spare string) (*Recovery, error) {
	hA, err := readHeader(aRoot, SideA)
	if err != nil {
		return nil, fmt.Errorf("share A: %w", err)
	}
	hS, err := readHeader(spare, SideB)
	if err != nil {
		return nil, fmt.Errorf("spare card: %w", err)
	}
	if hS.Role != roleBackup && hA.Role != roleBackup {
		return nil, errors.New("this is the working card, not a spare card; recovery is not needed")
	}
	if hA.Vault != hS.Vault {
		return nil, ErrWrongCard
	}
	id, err := ParseID(hA.Vault)
	if err != nil {
		return nil, err
	}
	r := &Recovery{aRoot: aRoot, spare: spare, id: id, Media: hS.Media, aCopy: hA.Role == roleBackup, bCopy: hS.Role == roleBackup}
	if !r.aCopy {
		if r.lock, err = lockRoot(aRoot); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Recovery) Close() {
	if r.lock != nil {
		r.lock.Close()
		r.lock = nil
	}
}

// Points returns READY points (newest first) and descriptions of rejected ones.
// When both sides are copies, only points present on both.
func (r *Recovery) Points() ([]Point, []string) {
	switch {
	case r.aCopy && r.bCopy:
		pb, bad := ReadPoints(r.spare, r.id)
		pa, badA := ReadPoints(r.aRoot, r.id)
		inA := map[string]bool{}
		for _, p := range pa {
			inA[p.ID] = true
		}
		var both []Point
		for _, p := range pb {
			if inA[p.ID] {
				both = append(both, p)
			} else {
				bad = append(bad, fmt.Sprintf("point %s: no share A copy", p.ID))
			}
		}
		return both, append(bad, badA...)
	case r.bCopy:
		return ReadPoints(r.spare, r.id)
	default:
		return ReadPoints(r.aRoot, r.id)
	}
}

func (r *Recovery) point(id string) (Point, error) {
	points, _ := r.Points()
	if len(points) == 0 {
		return Point{}, ErrNoPoint
	}
	if id == "" {
		return points[0], nil
	}
	for _, p := range points {
		if p.ID == id {
			return p, nil
		}
	}
	return Point{}, fmt.Errorf("%w with ID %s", ErrNoPoint, id)
}

// View opens the directory state from a recovery point (empty id: the newest).
func (r *Recovery) View(pointID string) (*View, Point, error) {
	p, err := r.point(pointID)
	if err != nil {
		return nil, p, err
	}
	ix, err := loadIndexFrom(r.aRoot, r.spare, r.id, p.Index)
	if err != nil {
		return nil, p, fmt.Errorf("index of point %s: %w", p.ID, err)
	}
	allowed := map[string]bool{p.Index: true}
	ix.refs(allowed)
	read := func(oid ID) ([]byte, error) {
		if !allowed[oid.String()] {
			return nil, fmt.Errorf("%w: object %s outside the recovery point", ErrCorrupt, oid)
		}
		return readPair(r.aRoot, r.spare, r.id, oid)
	}
	return &View{ix: ix, read: read}, p, nil
}

// Verify joins every file of the point in RAM and checks completeness (R-03).
func (r *Recovery) Verify(pointID string) (Point, int, error) {
	w, p, err := r.View(pointID)
	if err != nil {
		return p, 0, err
	}
	n := 0
	for _, path := range w.Paths() {
		e, _ := w.Stat(path)
		if e.Dir {
			continue
		}
		data, err := w.GetFile(path)
		if err != nil {
			return p, n, fmt.Errorf("file no. %d: %w", e.Ino, err)
		}
		split.Wipe(data)
		n++
	}
	return p, n, nil
}

// ExportTo writes plain files from the reader to a new directory. The resulting
// files are NOT protected (rollback S004 §8).
func ExportTo(r FileReader, dir string) (int, error) {
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		return 0, fmt.Errorf("%s already exists", dir)
	}
	n := 0
	for _, p := range r.Paths() {
		e, _ := r.Stat(p)
		if p != Clean(p) {
			return n, fmt.Errorf("%w: non-canonical path", ErrCorrupt)
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if rel, err := filepath.Rel(dir, dst); err != nil || strings.HasPrefix(rel, "..") {
			return n, fmt.Errorf("%w: path outside the export directory", ErrCorrupt)
		}
		if e.Dir {
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return n, err
			}
			continue
		}
		data, err := r.GetFile(p)
		if err != nil {
			return n, err
		}
		err = os.WriteFile(dst, data, 0o600)
		split.Wipe(data)
		if err != nil {
			return n, err
		}
		os.Chtimes(dst, e.ModTime, e.ModTime)
		n++
	}
	return n, nil
}

// CopyInto rewrites a file tree into another vault. Every block gets
// a new mask, so a lost card does not match the new shares (R-04).
func CopyInto(r FileReader, dst *Vault) (int, error) {
	n := 0
	for _, p := range r.Paths() {
		if p == "/" {
			continue
		}
		e, _ := r.Stat(p)
		if e.Dir {
			if err := dst.Mkdir(p); err != nil {
				return n, err
			}
			continue
		}
		data, err := r.GetFile(p)
		if err != nil {
			return n, err
		}
		err = dst.PutFile(p, data)
		split.Wipe(data)
		if err != nil {
			return n, err
		}
		if de, ok := dst.Stat(p); ok {
			dst.SetModTime(de.Ino, e.ModTime)
		}
		n++
	}
	return n, nil
}
