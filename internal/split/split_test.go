package split

import (
	"bytes"
	"testing"
)

// CG-01 (warstwa podziału): Join(Split(m)) == m dla różnych rozmiarów.
func TestRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 7, 4096, 65536, 65537} {
		m := bytes.Repeat([]byte("xordock!"), n/8+1)[:n]
		a, b, err := Split(m, CSPRNG{})
		if err != nil {
			t.Fatal(err)
		}
		got, err := Join(a, b)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, m) {
			t.Fatalf("n=%d: round trip się nie zgadza", n)
		}
	}
}

// CG-02/CG-03 (warstwa podziału): żaden udział nie zawiera jawnej treści.
func TestSharesDoNotContainPlaintext(t *testing.T) {
	m := bytes.Repeat([]byte("TAJNY-MARKER-0123456789"), 1000)
	a, b, err := Split(m, CSPRNG{})
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("TAJNY-MARKER")
	if bytes.Contains(a, marker) || bytes.Contains(b, marker) {
		t.Fatal("udział zawiera jawny marker")
	}
}

// CG-04 (warstwa podziału): dwa podziały tych samych danych mają różne maski.
func TestFreshMaskEachSplit(t *testing.T) {
	m := make([]byte, 4096)
	_, b1, _ := Split(m, CSPRNG{})
	_, b2, _ := Split(m, CSPRNG{})
	if bytes.Equal(b1, b2) {
		t.Fatal("ta sama maska użyta dwa razy")
	}
}

func TestJoinLengthMismatch(t *testing.T) {
	if _, err := Join([]byte{1, 2}, []byte{1}); err != ErrLength {
		t.Fatalf("oczekiwano ErrLength, jest %v", err)
	}
}

// CG-10: opis źródła nie może sugerować odporności na P = NP.
func TestKindIsHonest(t *testing.T) {
	k := Kind(CSPRNG{})
	if !bytes.Contains([]byte(k), []byte("BEZ gwarancji")) {
		t.Fatalf("opis CSPRNG nie mówi wprost o braku gwarancji: %q", k)
	}
}
