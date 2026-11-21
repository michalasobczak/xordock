package vault

import (
	"fmt"
	"os"
)

// FsckReport opisuje spójność sejfu bez ujawniania treści: weryfikujemy
// sumy obu udziałów każdego obiektu, ale ich nie składamy.
type FsckReport struct {
	Seq       uint64
	Files     int
	Dirs      int
	Bytes     int64
	Objects   int
	Problems  []string
	Recovered string
}

func (r FsckReport) OK() bool { return len(r.Problems) == 0 }

func (v *Vault) Fsck() FsckReport {
	v.mu.Lock()
	defer v.mu.Unlock()
	r := FsckReport{Seq: v.cur.Seq, Recovered: v.Recovery}
	seen := map[string]bool{}
	check := func(hexID string) {
		if seen[hexID] {
			return
		}
		seen[hexID] = true
		r.Objects++
		id, err := ParseID(hexID)
		if err != nil {
			r.Problems = append(r.Problems, err.Error())
			return
		}
		for _, s := range []struct {
			root string
			side Side
		}{{v.aRoot, SideA}, {v.bRoot, SideB}} {
			raw, err := os.ReadFile(objPath(s.root, id))
			if err == nil {
				_, err = decodeShare(raw, s.side, v.id, id)
			}
			if err != nil {
				r.Problems = append(r.Problems, fmt.Sprintf("obiekt %s, udział %c: %v", hexID, s.side, err))
			}
		}
	}
	check(v.cur.Index)
	for _, e := range v.idx.Entries {
		if e.Dir {
			r.Dirs++
			continue
		}
		r.Files++
		r.Bytes += e.Size
		if want := (e.Size + BlockSize - 1) / BlockSize; int64(len(e.Blocks)) != want {
			r.Problems = append(r.Problems, fmt.Sprintf("plik ma %d bloków zamiast %d", len(e.Blocks), want))
		}
		for _, b := range e.Blocks {
			check(b)
		}
	}
	return r
}
