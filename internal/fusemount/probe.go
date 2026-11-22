package fusemount

import (
	"fmt"
	"os"
	"time"

	"xordock/internal/fusefs"
)

// Probe sprawdza, czy macFUSE z backendem FSKit faktycznie montuje (S011
// W-03): montuje pusty wolumen bez sejfu w katalogu tymczasowym i od razu
// go odmontowuje. Stanu rozszerzenia FSKit nie da się odczytać inaczej
// (chroniony kontener systemu).
func Probe(timeout time.Duration) error {
	if !Available() {
		return ErrNoMacFUSE
	}
	mp, err := os.MkdirTemp("", "xordock-sprawdzenie-")
	if err != nil {
		return err
	}
	defer os.Remove(mp)
	errc := make(chan error, 1)
	go func() {
		errc <- run(fusefs.NewProbe(), mp, "backend=fskit,volname=xordock-sprawdzenie,fsname=xordock")
	}()
	deadline := time.After(timeout)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-errc:
			if err == nil {
				err = errMount // pętla skończyła się, zanim wolumen się pojawił
			}
			return err
		case <-tick.C:
			if IsMounted(mp) {
				unmount(mp, false)
				select {
				case <-errc:
				case <-time.After(10 * time.Second):
					exitLoop()
				}
				return nil
			}
		case <-deadline:
			unmount(mp, true)
			exitLoop()
			return fmt.Errorf("%w (brak wolumenu po %v)", errMount, timeout)
		}
	}
}
