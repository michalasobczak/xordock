package main

import (
	"flag"
	"fmt"
	"os/exec"
	"strings"

	"xordock/internal/fusemount"
)

func mountpoint(name, at string) string {
	if at != "" {
		return at
	}
	return "/Volumes/xordock-" + name
}

// cmdMount mounts the vault in Finder and runs until unmounted (F-02, F-04, F-05).
func cmdMount(args []string) error {
	fl := flag.NewFlagSet("mount", flag.ExitOnError)
	var c common
	c.bind(fl)
	at := fl.String("at", "", "mount point (default /Volumes/xordock-<name>)")
	fl.Parse(args)
	if out, err := exec.Command("/usr/bin/fdesetup", "status").Output(); err != nil || !strings.Contains(string(out), "On") {
		fmt.Println("WARNING: FileVault is off. Apps may leave plain copies on the SSD (D-07); FileVault is recommended.")
	}
	v, err := open(c)
	if err != nil {
		return err
	}
	defer v.Close()
	mp := mountpoint(c.name, *at)
	onMounted := func() {
		fmt.Printf("Vault %q mounted at %s (macFUSE, FSKit backend).\n", c.name, mp)
		fmt.Println("Masks:", v.MaskKind())
		fmt.Println("To unmount: eject the volume in Finder, run xordock unmount, or press Ctrl-C. Removing the card locks and unmounts the volume.")
	}
	if err := fusemount.Mount(v, mp, "xordock-"+c.name, onMounted); err != nil {
		return err
	}
	if err := v.Broken(); err != nil {
		return fmt.Errorf("volume unmounted in an emergency, uncommitted changes were lost: %w", err)
	}
	fmt.Println("Unmounted. The card can be safely ejected.")
	return nil
}

func cmdUnmount(args []string) error {
	fl := flag.NewFlagSet("unmount", flag.ExitOnError)
	var c common
	c.bind(fl)
	at := fl.String("at", "", "mount point")
	fl.Parse(args)
	return fusemount.Unmount(mountpoint(c.name, *at))
}
