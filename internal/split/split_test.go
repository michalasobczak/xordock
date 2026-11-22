package split

import (
	"bytes"
	"testing"
)

// CG-01 (split layer): Join(Split(m)) == m for various sizes.
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
			t.Fatalf("n=%d: round trip does not match", n)
		}
	}
}

// CG-02/CG-03 (split layer): no share contains plain content.
func TestSharesDoNotContainPlaintext(t *testing.T) {
	m := bytes.Repeat([]byte("SECRET-MARKER-0123456789"), 1000)
	a, b, err := Split(m, CSPRNG{})
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("SECRET-MARKER")
	if bytes.Contains(a, marker) || bytes.Contains(b, marker) {
		t.Fatal("share contains the plain marker")
	}
}

// CG-04 (split layer): two splits of the same data use different masks.
func TestFreshMaskEachSplit(t *testing.T) {
	m := make([]byte, 4096)
	_, b1, _ := Split(m, CSPRNG{})
	_, b2, _ := Split(m, CSPRNG{})
	if bytes.Equal(b1, b2) {
		t.Fatal("the same mask used twice")
	}
}

func TestJoinLengthMismatch(t *testing.T) {
	if _, err := Join([]byte{1, 2}, []byte{1}); err != ErrLength {
		t.Fatalf("expected ErrLength, got %v", err)
	}
}

// CG-10: the source description must not suggest resistance to P = NP.
func TestKindIsHonest(t *testing.T) {
	k := Kind(CSPRNG{})
	if !bytes.Contains([]byte(k), []byte("NO information-theoretic guarantee")) {
		t.Fatalf("CSPRNG description does not state the missing guarantee explicitly: %q", k)
	}
}

// testOnlyMasks is a deterministic TEST_ONLY source (S004 D-06). It lives in a
// _test.go file, so it cannot end up in the release binary.
type testOnlyMasks struct{ next byte }

func (s *testOnlyMasks) Fill(b []byte) error {
	for i := range b {
		b[i] = s.next
		s.next++
	}
	return nil
}

// D-06: splitting works with any MaskSource; a deterministic mask gives
// repeatable shares (tests only).
func TestTestOnlySourceDeterministic(t *testing.T) {
	m := []byte("abc")
	a1, _, _ := Split(m, &testOnlyMasks{})
	a2, _, _ := Split(m, &testOnlyMasks{})
	if !bytes.Equal(a1, a2) {
		t.Fatal("deterministic source gave different shares")
	}
}

type failingMasks struct{}

func (failingMasks) Fill([]byte) error { return ErrLength }

// D-06: a generator error stops the split.
func TestMaskErrorStopsSplit(t *testing.T) {
	if _, _, err := Split([]byte("x"), failingMasks{}); err == nil {
		t.Fatal("mask source error did not stop the split")
	}
}
