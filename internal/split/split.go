// Package split splits data into two XOR shares and joins them back.
//
//	A = M XOR R
//	B = R
//	M = A XOR B
//
// R comes from a MaskSource. In the MVP it is the macOS system CSPRNG
// (SecRandomCopyBytes, S004 D-06), which gives practical protection, NOT
// information-theoretic secrecy (resistant to P = NP). See S004 G-09, CG-14.
package split

import "errors"

// MaskSource supplies a fresh mask. Every Fill call must produce new,
// independent bytes; the implementation must not remember or log them.
type MaskSource interface {
	Fill(b []byte) error
}

// CSPRNG is the macOS system generator (SecRandomCopyBytes, csprng_darwin.go).
// A generator error stops the write (S004 D-06).
type CSPRNG struct{}

// Kind describes the guarantee class of the mask source. Used in the UI so
// that the CSPRNG is not presented as resistant to P = NP (CG-10).
func Kind(MaskSource) string {
	return "macOS CSPRNG (SecRandomCopyBytes): practical (computational) protection, NO information-theoretic guarantee and no resistance to P = NP"
}

var ErrLength = errors.New("shares have different lengths")

// Split returns (a, b) for data m with a new mask.
func Split(m []byte, src MaskSource) (a, b []byte, err error) {
	b = make([]byte, len(m))
	if err := src.Fill(b); err != nil {
		return nil, nil, err
	}
	a = make([]byte, len(m))
	for i := range m {
		a[i] = m[i] ^ b[i]
	}
	return a, b, nil
}

// Join restores m = a XOR b.
func Join(a, b []byte) ([]byte, error) {
	if len(a) != len(b) {
		return nil, ErrLength
	}
	m := make([]byte, len(a))
	for i := range a {
		m[i] = a[i] ^ b[i]
	}
	return m, nil
}

// Wipe zeroes a buffer holding plain data (best effort: the Go GC may keep copies).
func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
