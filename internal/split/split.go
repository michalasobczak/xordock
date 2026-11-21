// Package split dzieli dane na dwa udziały XOR i składa je z powrotem.
//
//	A = M XOR R
//	B = R
//	M = A XOR B
//
// R pochodzi z MaskSource. W prototypie jest to CSPRNG systemu (crypto/rand),
// co NIE daje poufności bezwarunkowej (odpornej na P = NP). Zob. S001, IT-01–IT-04.
package split

import (
	"crypto/rand"
	"errors"
	"io"
)

// MaskSource dostarcza świeżą maskę. Każde wywołanie Fill musi dać nowe,
// niezależne bajty; implementacja nie może ich zapamiętywać ani logować.
type MaskSource interface {
	Fill(b []byte) error
}

// CSPRNG to maska z crypto/rand (na macOS: getentropy / arc4random).
type CSPRNG struct{}

func (CSPRNG) Fill(b []byte) error {
	_, err := io.ReadFull(rand.Reader, b)
	return err
}

// Kind opisuje klasę gwarancji źródła masek. Używany w interfejsie, żeby
// nie przedstawiać CSPRNG jako odpornego na P = NP (CG-10).
func Kind(MaskSource) string {
	return "CSPRNG (prototyp): poufność obliczeniowa, BEZ gwarancji odporności na P = NP"
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
