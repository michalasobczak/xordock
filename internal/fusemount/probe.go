package fusemount

import (
	"fmt"
	"os"
	"time"

	"xordock/internal/fusefs"
)

// Probe checks whether macFUSE with the FSKit backend actually mounts (S011
// W-03): it mounts an empty volume without a vault in a temporary directory and
// unmounts it right away. The FSKit extension state cannot be read any other way
// (protected system container).
func Probe(timeout time.Duration) error {
	if !Available() {
		return ErrNoMacFUSE
	}
	mp, err := os.MkdirTemp("", "xordock-probe-")
	if err != nil {
		return err
	}
	defer os.Remove(mp)
	errc := make(chan error, 1)
	go func() {
		errc <- run(fusefs.NewProbe(), mp, "backend=fskit,volname=xordock-probe,fsname=xordock")
	}()
	deadline := time.After(timeout)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-errc:
			if err == nil {
				err = errMount // the loop ended before the volume appeared
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
			return fmt.Errorf("%w (no volume after %v)", errMount, timeout)
		}
	}
}
