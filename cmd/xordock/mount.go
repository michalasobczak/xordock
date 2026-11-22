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

// cmdMount montuje sejf w Finderze i działa do odmontowania (F-02, F-04, F-05).
func cmdMount(args []string) error {
	fl := flag.NewFlagSet("mount", flag.ExitOnError)
	var c common
	c.bind(fl)
	at := fl.String("at", "", "punkt montowania (domyślnie /Volumes/xordock-<name>)")
	fl.Parse(args)
	if out, err := exec.Command("/usr/bin/fdesetup", "status").Output(); err != nil || !strings.Contains(string(out), "On") {
		fmt.Println("UWAGA: FileVault wyłączony. Aplikacje mogą zostawić jawne kopie na SSD (D-07); zalecamy FileVault.")
	}
	v, err := open(c)
	if err != nil {
		return err
	}
	defer v.Close()
	mp := mountpoint(c.name, *at)
	onMounted := func() {
		fmt.Printf("Sejf %q zamontowany w %s (macFUSE, backend FSKit).\n", c.name, mp)
		fmt.Println("Maski:", v.MaskKind())
		fmt.Println("Odmontowanie: wysuń wolumen w Finderze, xordock unmount albo Ctrl-C. Wyjęcie karty blokuje i odmontowuje wolumen.")
	}
	if err := fusemount.Mount(v, mp, "xordock-"+c.name, onMounted); err != nil {
		return err
	}
	if err := v.Broken(); err != nil {
		return fmt.Errorf("wolumen odmontowany awaryjnie, niezatwierdzone zmiany przepadły: %w", err)
	}
	fmt.Println("Odmontowano. Kartę można bezpiecznie wysunąć.")
	return nil
}

func cmdUnmount(args []string) error {
	fl := flag.NewFlagSet("unmount", flag.ExitOnError)
	var c common
	c.bind(fl)
	at := fl.String("at", "", "punkt montowania")
	fl.Parse(args)
	return fusemount.Unmount(mountpoint(c.name, *at))
}
