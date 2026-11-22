package split

/*
#cgo LDFLAGS: -framework Security
#include <Security/SecRandom.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Fill fills b with bytes from SecRandomCopyBytes and checks the result code.
func (CSPRNG) Fill(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if st := C.SecRandomCopyBytes(C.kSecRandomDefault, C.size_t(len(b)), unsafe.Pointer(&b[0])); st != 0 {
		return fmt.Errorf("SecRandomCopyBytes: code %d", int(st))
	}
	return nil
}
