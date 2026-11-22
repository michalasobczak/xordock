//go:build !darwin

package split

import "errors"

// Fill refuses outside macOS: the product supports only macOS (S004 §4).
func (CSPRNG) Fill(b []byte) error {
	return errors.New("mask source is available only on macOS")
}
