package vault

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Share format, version 1.
//
// Each root (A on the SSD, B on the card) has:
//
//	xordock.json      header: format, vault ID, side, name, role
//	super             superblock: current and previous commit (+ SHA-256 checksum)
//	obj/xx/<oid>      object share (data block or index)
//
// A spare card (role "backup", S004 §5.5) has the same header (side B),
// an obj/ directory with copies of B shares and a points/ directory with recovery points.
// It has no superblock: it cannot be opened as a working card (CG-09).
//
// Object share file:
//
//	"XDK1" | side(1) | vault(16) | oid(16) | length(uint32 BE) | data | SHA-256(everything before)
//
// The checksum covers only its own share, never the other. A hash of share A
// stored on B (or vice versa) would let an unbounded adversary
// holding one share verify hypotheses about the content, which breaks G-06.
// The checksum detects accidental damage, not deliberate tampering (S001 Q-05, R-04).

const (
	FormatVersion = 1
	BlockSize     = 1 << 20 // 1 MiB: fewer fsync calls per MiB (measured in S001 §8)
	magic         = "XDK1"
	headerFile    = "xordock.json"
	superFile     = "super"
	objDir        = "obj"
	idLen         = 16
	prefixLen     = 4 + 1 + idLen + idLen + 4
	// MaxObject limits the size of a single object (the index may be
	// larger than a block). Protects against allocation from a damaged header.
	MaxObject = 256 << 20
)

var (
	ErrCorrupt      = errors.New("share damaged or incompatible")
	ErrWrongCard    = errors.New("share B belongs to a different vault")
	ErrInconsistent = errors.New("superblocks A and B are inconsistent and cannot be safely reconciled")
	ErrBroken       = errors.New("vault is in an error state after a failed write; unmount and open it again")
	ErrSpareCard    = errors.New("this is a spare card (backup), not the working card; for recovery use: xordock recover")
)

type ID [idLen]byte

func (id ID) String() string { return hex.EncodeToString(id[:]) }

func ParseID(s string) (ID, error) {
	var id ID
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != idLen {
		return id, fmt.Errorf("%w: bad ID", ErrCorrupt)
	}
	copy(id[:], b)
	return id, nil
}

func newID() (ID, error) {
	var id ID
	_, err := rand.Read(id[:])
	return id, err
}

type Side byte

const (
	SideA Side = 'A'
	SideB Side = 'B'
)

type header struct {
	Format    int       `json:"format"`
	Vault     string    `json:"vault"`
	Side      string    `json:"side"`
	Name      string    `json:"name"`
	BlockSize int       `json:"block_size"`
	Created   time.Time `json:"created"`
	Masks     string    `json:"masks"`
	Role      string    `json:"role,omitempty"`  // "" = working card, "backup" = spare
	Media     string    `json:"media,omitempty"` // spare medium ID
}

const roleBackup = "backup"

func readHeader(root string, side Side) (header, error) {
	var h header
	data, err := os.ReadFile(filepath.Join(root, headerFile))
	if err != nil {
		return h, err
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return h, fmt.Errorf("%w: header %c: %v", ErrCorrupt, side, err)
	}
	if h.Format != FormatVersion {
		return h, fmt.Errorf("%w: unsupported format %d", ErrCorrupt, h.Format)
	}
	if h.Side != string(side) {
		return h, fmt.Errorf("%w: expected share %c, got %q", ErrCorrupt, side, h.Side)
	}
	return h, nil
}

// commitRec points to a vault state: commit number and index object.
type commitRec struct {
	Seq   uint64 `json:"seq"`
	Index string `json:"index"`
}

type superblock struct {
	Format int       `json:"format"`
	Vault  string    `json:"vault"`
	Side   string    `json:"side"`
	Cur    commitRec `json:"cur"`
	Prev   commitRec `json:"prev"`
}

// seal writes a JSON record with a SHA-256 checksum on the second line (superblock,
// recovery point). The checksum detects accidental damage, not tampering.
func seal(v any) []byte {
	body, _ := json.Marshal(v)
	sum := sha256.Sum256(body)
	return append(append(body, '\n'), []byte(hex.EncodeToString(sum[:])+"\n")...)
}

// unseal checks the checksum and decodes a record written by seal.
func unseal(data []byte, into any) error {
	parts := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(parts) != 2 {
		return ErrCorrupt
	}
	sum := sha256.Sum256([]byte(parts[0]))
	if hex.EncodeToString(sum[:]) != parts[1] {
		return ErrCorrupt
	}
	if err := json.Unmarshal([]byte(parts[0]), into); err != nil {
		return ErrCorrupt
	}
	return nil
}

func encodeSuper(s superblock) []byte { return seal(s) }

func readSuper(root string, side Side, vault ID) (superblock, error) {
	var s superblock
	data, err := os.ReadFile(filepath.Join(root, superFile))
	if err != nil {
		return s, err
	}
	if err := unseal(data, &s); err != nil {
		return s, fmt.Errorf("%w: superblock %c", ErrCorrupt, side)
	}
	if s.Vault != vault.String() {
		return s, ErrWrongCard
	}
	if s.Side != string(side) {
		return s, fmt.Errorf("%w: superblock from the wrong side", ErrCorrupt)
	}
	return s, nil
}

func objPath(root string, oid ID) string {
	h := oid.String()
	return filepath.Join(root, objDir, h[:2], h)
}

func encodeShare(side Side, vault, oid ID, payload []byte) []byte {
	buf := make([]byte, 0, prefixLen+len(payload)+sha256.Size)
	buf = append(buf, magic...)
	buf = append(buf, byte(side))
	buf = append(buf, vault[:]...)
	buf = append(buf, oid[:]...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(payload)))
	buf = append(buf, payload...)
	sum := sha256.Sum256(buf)
	return append(buf, sum[:]...)
}

// decodeShare verifies a share and returns the data. Any mismatch (side,
// vault, ID, length, checksum) is an error, never random data (FR-14).
func decodeShare(data []byte, side Side, vault, oid ID) ([]byte, error) {
	if len(data) < prefixLen+sha256.Size {
		return nil, fmt.Errorf("%w: share too short %c/%s", ErrCorrupt, side, oid)
	}
	body, sum := data[:len(data)-sha256.Size], data[len(data)-sha256.Size:]
	want := sha256.Sum256(body)
	if !bytes.Equal(want[:], sum) {
		return nil, fmt.Errorf("%w: share checksum %c/%s", ErrCorrupt, side, oid)
	}
	if string(body[:4]) != magic || Side(body[4]) != side ||
		!bytes.Equal(body[5:5+idLen], vault[:]) || !bytes.Equal(body[5+idLen:5+2*idLen], oid[:]) {
		return nil, fmt.Errorf("%w: share header %c/%s", ErrCorrupt, side, oid)
	}
	n := binary.BigEndian.Uint32(body[5+2*idLen : prefixLen])
	if int(n) != len(body)-prefixLen {
		return nil, fmt.Errorf("%w: share length %c/%s", ErrCorrupt, side, oid)
	}
	return body[prefixLen:], nil
}
