// xordock: chroniony katalog macOS przechowywany jako dwa udziały XOR,
// A na SSD Maca i B na karcie SD. Zob. specs/S001-xordock-secure-folders.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"xordock/internal/split"
	"xordock/internal/vault"
)

const usage = `xordock — katalog w Finderze, dane podzielone XOR między SSD i kartę SD

Użycie:
  xordock init   --card /Volumes/KARTA [--name sejf] [--a KATALOG]
  xordock fsck   [--name sejf] [--a KATALOG]
  xordock export [--name sejf] [--a KATALOG] --to KATALOG

Udział A domyślnie: ~/Library/Application Support/xordock/<name>
Udział B: <karta>/xordock/<name>, rozpoznawany po identyfikatorze sejfu.
Montowanie w Finderze: rozszerzenie FSKit (S002, w budowie).
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "fsck":
		err = cmdFsck(os.Args[2:])
	case "export":
		err = cmdExport(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "xordock:", err)
		os.Exit(1)
	}
}

func home() string { h, _ := os.UserHomeDir(); return h }

type common struct {
	name, a string
}

func (c *common) bind(fl *flag.FlagSet) {
	fl.StringVar(&c.name, "name", "sejf", "nazwa sejfu")
	fl.StringVar(&c.a, "a", "", "katalog udziału A (domyślnie w Application Support)")
}

func (c *common) aRoot() string {
	if c.a != "" {
		return c.a
	}
	return filepath.Join(home(), "Library", "Application Support", "xordock", c.name)
}

func warnings() {
	fmt.Println("Maski:", split.Kind(split.CSPRNG{}))
}

func cmdInit(args []string) error {
	fl := flag.NewFlagSet("init", flag.ExitOnError)
	var c common
	c.bind(fl)
	card := fl.String("card", "", "wolumen karty SD, np. /Volumes/xordock")
	fl.Parse(args)
	if *card == "" {
		return errors.New("podaj --card")
	}
	if !strings.HasPrefix(filepath.Clean(*card), "/Volumes/") {
		return errors.New("--card musi wskazywać wolumen w /Volumes")
	}
	bRoot := filepath.Join(*card, "xordock", c.name)
	v, err := vault.Create(c.aRoot(), bRoot, c.name, vault.Options{})
	if err != nil {
		return err
	}
	fmt.Printf("Utworzono sejf %q (%s)\n  A: %s\n  B: %s\n", c.name, v.ID(), c.aRoot(), bRoot)
	warnings()
	fmt.Println("Bez karty danych nie da się odtworzyć. Utrata karty = utrata danych (zrób kopię katalogu z karty na drugą kartę).")
	return nil
}

// findB szuka udziału B na podłączonych wolumenach po identyfikatorze
// sejfu, nie po nazwie wolumenu (FR-16).
func findB(aRoot string) (string, error) {
	id, name, err := vault.ReadVaultID(aRoot)
	if err != nil {
		return "", fmt.Errorf("udział A: %w", err)
	}
	matches, _ := filepath.Glob("/Volumes/*/xordock/*/xordock.json")
	for _, m := range matches {
		dir := filepath.Dir(m)
		data, err := os.ReadFile(m)
		if err == nil && strings.Contains(string(data), `"`+id.String()+`"`) {
			return dir, nil
		}
	}
	return "", fmt.Errorf("nie znaleziono karty sejfu %q (%s); podłącz właściwą kartę", name, id)
}

func open(c common) (*vault.Vault, error) {
	b, err := findB(c.aRoot())
	if err != nil {
		return nil, err
	}
	v, err := vault.Open(c.aRoot(), b, vault.Options{})
	if err != nil {
		return nil, err
	}
	if v.Recovery != "" {
		fmt.Println("Odzyskiwanie:", v.Recovery)
	}
	return v, nil
}

func cmdFsck(args []string) error {
	fl := flag.NewFlagSet("fsck", flag.ExitOnError)
	var c common
	c.bind(fl)
	fl.Parse(args)
	v, err := open(c)
	if err != nil {
		return err
	}
	r := v.Fsck()
	fmt.Printf("commit %d: %d plików, %d katalogów, %d B danych, %d obiektów\n", r.Seq, r.Files, r.Dirs, r.Bytes, r.Objects)
	for _, p := range r.Problems {
		fmt.Println("BŁĄD:", p)
	}
	if !r.OK() {
		return fmt.Errorf("wykryto %d problemów", len(r.Problems))
	}
	fmt.Println("OK")
	return nil
}

func cmdExport(args []string) error {
	fl := flag.NewFlagSet("export", flag.ExitOnError)
	var c common
	c.bind(fl)
	to := fl.String("to", "", "katalog docelowy (nie może istnieć)")
	fl.Parse(args)
	if *to == "" {
		return errors.New("podaj --to")
	}
	if _, err := os.Stat(*to); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s już istnieje", *to)
	}
	v, err := open(c)
	if err != nil {
		return err
	}
	fmt.Println("UWAGA: wyeksportowane pliki NIE są chronione podziałem XOR.")
	n := 0
	for _, p := range v.Paths() {
		e, _ := v.Stat(p)
		dst := filepath.Join(*to, filepath.FromSlash(p))
		if e.Dir {
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return err
			}
			continue
		}
		data, err := v.GetFile(p)
		if err != nil {
			return err
		}
		err = os.WriteFile(dst, data, 0o600)
		split.Wipe(data)
		if err != nil {
			return err
		}
		n++
	}
	fmt.Printf("Wyeksportowano %d plików do %s\n", n, *to)
	return nil
}
