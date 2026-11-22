// Package split dzieli dane na dwa udziały XOR i składa je z powrotem.
//
//	A = M XOR R
//	B = R
//	M = A XOR B
//
// R pochodzi z MaskSource. W MVP jest to CSPRNG systemu macOS
// (SecRandomCopyBytes, S004 D-06), co daje ochronę praktyczną, a NIE
// poufność informacyjno-teoretyczną (odporną na P = NP). Zob. S004 G-09, CG-14.
package split

import "errors"

// MaskSource dostarcza świeżą maskę. Każde wywołanie Fill musi dać nowe,
// niezależne bajty; implementacja nie może ich zapamiętywać ani logować.
type MaskSource interface {
	Fill(b []byte) error
}

// CSPRNG to systemowy generator macOS (SecRandomCopyBytes, csprng_darwin.go).
// Błąd generatora zatrzymuje zapis (S004 D-06).
type CSPRNG struct{}

// Kind opisuje klasę gwarancji źródła masek. Używany w interfejsie, żeby
// nie przedstawiać CSPRNG jako odpornego na P = NP (CG-10).
func Kind(MaskSource) string {
	return "CSPRNG macOS (SecRandomCopyBytes): ochrona praktyczna (obliczeniowa), BEZ gwarancji informacyjno-teoretycznej i odporności na P = NP"
}

var ErrLength = errors.New("udziały mają różną długość")

// Split zwraca (a, b) dla danych m z nową maską.
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

// Join odtwarza m = a XOR b.
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

// Wipe zeruje bufor z jawnymi danymi (best effort: GC Go może mieć kopie).
func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
