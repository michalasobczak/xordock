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

// Format udziałów, wersja 1.
//
// Każdy korzeń (A na SSD, B na karcie) ma:
//
//	xordock.json      nagłówek: format, identyfikator sejfu, strona, nazwa
//	super             superblok: bieżący i poprzedni commit (+ suma SHA-256)
//	obj/xx/<oid>      udział obiektu (bloku danych albo indeksu)
//
// Plik udziału obiektu:
//
//	"XDK1" | strona(1) | vault(16) | oid(16) | długość(uint32 BE) | dane | SHA-256(wszystko wcześniej)
//
// Suma kontrolna obejmuje tylko własny udział, nigdy drugi. Skrót udziału A
// zapisany na B (lub odwrotnie) pozwoliłby nieograniczonemu przeciwnikowi
// z jednym udziałem weryfikować hipotezy o treści, co łamie G-06.
// Suma wykrywa przypadkowe uszkodzenia, nie celową podmianę (S001 Q-05, R-04).

const (
	FormatVersion = 1
	BlockSize     = 1 << 20 // 1 MiB: mniej wywołań fsync na MiB (pomiar w S001 §8)
	magic         = "XDK1"
	headerFile    = "xordock.json"
	superFile     = "super"
	objDir        = "obj"
	idLen         = 16
	prefixLen     = 4 + 1 + idLen + idLen + 4
	// MaxObject ogranicza rozmiar pojedynczego obiektu (indeks może być
	// większy niż blok). Chroni przed alokacją z uszkodzonego nagłówka.
	MaxObject = 256 << 20
)

var (
	ErrCorrupt      = errors.New("udział uszkodzony albo niezgodny")
	ErrWrongCard    = errors.New("udział B należy do innego sejfu")
	ErrInconsistent = errors.New("superbloki A i B są niespójne i nie da się ich bezpiecznie uzgodnić")
	ErrBroken       = errors.New("sejf w stanie błędu po nieudanym zapisie; odmontuj i otwórz ponownie")
)

type ID [idLen]byte

func (id ID) String() string { return hex.EncodeToString(id[:]) }

func ParseID(s string) (ID, error) {
	var id ID
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != idLen {
		return id, fmt.Errorf("%w: zły identyfikator", ErrCorrupt)
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
}

func readHeader(root string, side Side) (header, error) {
	var h header
	data, err := os.ReadFile(filepath.Join(root, headerFile))
	if err != nil {
		return h, err
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return h, fmt.Errorf("%w: nagłówek %c: %v", ErrCorrupt, side, err)
	}
	if h.Format != FormatVersion {
		return h, fmt.Errorf("%w: nieobsługiwany format %d", ErrCorrupt, h.Format)
	}
	if h.Side != string(side) {
		return h, fmt.Errorf("%w: oczekiwano udziału %c, jest %q", ErrCorrupt, side, h.Side)
	}
	return h, nil
}

// commitRec wskazuje stan sejfu: numer commitu i obiekt indeksu.
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

func encodeSuper(s superblock) []byte {
	body, _ := json.Marshal(s)
	sum := sha256.Sum256(body)
	return append(append(body, '\n'), []byte(hex.EncodeToString(sum[:])+"\n")...)
}

func readSuper(root string, side Side, vault ID) (superblock, error) {
	var s superblock
	data, err := os.ReadFile(filepath.Join(root, superFile))
	if err != nil {
		return s, err
	}
	parts := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(parts) != 2 {
		return s, fmt.Errorf("%w: superblok %c", ErrCorrupt, side)
	}
	sum := sha256.Sum256([]byte(parts[0]))
	if hex.EncodeToString(sum[:]) != parts[1] {
		return s, fmt.Errorf("%w: suma superbloku %c", ErrCorrupt, side)
	}
	if err := json.Unmarshal([]byte(parts[0]), &s); err != nil {
		return s, fmt.Errorf("%w: superblok %c", ErrCorrupt, side)
	}
	if s.Vault != vault.String() {
		return s, ErrWrongCard
	}
	if s.Side != string(side) {
		return s, fmt.Errorf("%w: superblok ze złej strony", ErrCorrupt)
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

// decodeShare weryfikuje udział i zwraca dane. Każda niezgodność (strona,
// sejf, identyfikator, długość, suma) to błąd, nigdy przypadkowe dane (FR-14).
func decodeShare(data []byte, side Side, vault, oid ID) ([]byte, error) {
	if len(data) < prefixLen+sha256.Size {
		return nil, fmt.Errorf("%w: za krótki udział %c/%s", ErrCorrupt, side, oid)
	}
	body, sum := data[:len(data)-sha256.Size], data[len(data)-sha256.Size:]
	want := sha256.Sum256(body)
	if !bytes.Equal(want[:], sum) {
		return nil, fmt.Errorf("%w: suma udziału %c/%s", ErrCorrupt, side, oid)
	}
	if string(body[:4]) != magic || Side(body[4]) != side ||
		!bytes.Equal(body[5:5+idLen], vault[:]) || !bytes.Equal(body[5+idLen:5+2*idLen], oid[:]) {
		return nil, fmt.Errorf("%w: nagłówek udziału %c/%s", ErrCorrupt, side, oid)
	}
	n := binary.BigEndian.Uint32(body[5+2*idLen : prefixLen])
	if int(n) != len(body)-prefixLen {
		return nil, fmt.Errorf("%w: długość udziału %c/%s", ErrCorrupt, side, oid)
	}
	return body[prefixLen:], nil
}
