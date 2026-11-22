//go:build !darwin

package split

import "errors"

// Fill poza macOS odmawia: produkt wspiera tylko macOS (S004 §4).
func (CSPRNG) Fill(b []byte) error {
	return errors.New("źródło masek dostępne tylko na macOS")
}
