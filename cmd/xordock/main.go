// xordock: chroniony katalog macOS przechowywany jako dwa udziały XOR,
// A na SSD Maca i B na karcie SD. Zob. specs/S004-xordock-macos-mvp.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xordock/internal/split"
	"xordock/internal/vault"
)

const usage = `xordock — katalog w Finderze, dane podzielone XOR między SSD i kartę SD

Użycie (wspólne flagi: --name sejf, --a KATALOG_A):
  xordock init     --card /Volumes/KARTA
  xordock mount    [--at /Volumes/xordock-<name>]   katalog w Finderze (macFUSE, backend FSKit)
  xordock unmount  [--at …]
  xordock status                                    karta, migawki, ostatni backup, co może przepaść
  xordock fsck
  xordock export   --to KATALOG                     jawne pliki, NIE są już chronione
  xordock snapshot create | list | rm --id ID
  xordock backup   --spare /Volumes/ZAPASOWA [--snapshot ID] [--no-icloud]
                   B na kartę zapasową + A do iCloud Drive (ten sam punkt)
  xordock backup   prune --spare /Volumes/ZAPASOWA  zostawia tylko najnowszy punkt (karta i iCloud)
  xordock recover  --spare /Volumes/ZAPASOWA list | verify | export --to KATALOG |
                   restore --card /Volumes/NOWA [--new-name NAZWA]   [--point ID]
  xordock recover  --from-icloud --spare /Volumes/KARTA …   po utracie SSD: A z iCloud,
                   B z karty zapasowej albo roboczej

Zapytania dla aplikacji (JSON):
  xordock vaults | volumes | points --spare /Volumes/ZAPASOWA

Udział A domyślnie: ~/Library/Application Support/xordock/<name>
Udział B: <karta>/xordock/<name>, rozpoznawany po identyfikatorze sejfu.
Backup wymaga karty roboczej i zapasowej podłączonych jednocześnie.
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
	case "status":
		err = cmdStatus(os.Args[2:])
	case "snapshot":
		err = cmdSnapshot(os.Args[2:])
	case "backup":
		err = cmdBackup(os.Args[2:])
	case "recover":
		err = cmdRecover(os.Args[2:])
	case "mount":
		err = cmdMount(os.Args[2:])
	case "unmount":
		err = cmdUnmount(os.Args[2:])
	case "vaults":
		err = cmdVaults(os.Args[2:])
	case "volumes":
		err = cmdVolumes(os.Args[2:])
	case "points":
		err = cmdPoints(os.Args[2:])
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
	c.name = "sejf"
	fl.Func("name", "nazwa sejfu (domyślnie sejf)", func(s string) error {
		if err := validName(s); err != nil {
			return err
		}
		c.name = s
		return nil
	})
	fl.StringVar(&c.a, "a", "", "katalog udziału A (domyślnie w Application Support)")
}

func (c *common) aRoot() string {
	if c.a != "" {
		return c.a
	}
	return filepath.Join(home(), "Library", "Application Support", "xordock", c.name)
}

// validName: nazwa sejfu jest składnikiem ścieżek A, B i punktu montowania,
// więc nie może zawierać separatora ani zaczynać się kropką (S007, ta sama
// reguła co w aplikacji).
func validName(s string) error {
	switch {
	case s == "":
		return errors.New("pusta nazwa sejfu")
	case len([]rune(s)) > 64:
		return errors.New("nazwa sejfu dłuższa niż 64 znaki")
	case strings.HasPrefix(s, "."):
		return errors.New("nazwa sejfu nie może zaczynać się kropką")
	case strings.ContainsAny(s, "/:"):
		return errors.New("nazwa sejfu nie może zawierać „/” ani „:”")
	case strings.TrimSpace(s) != s:
		return errors.New("nazwa sejfu nie może zaczynać się ani kończyć spacją")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return errors.New("nazwa sejfu zawiera znak sterujący")
		}
	}
	return nil
}

// iCloudDrive: katalog iCloud Drive użytkownika (S008, miejsce na sztywno).
func iCloudDrive() string {
	return filepath.Join(home(), "Library", "Mobile Documents", "com~apple~CloudDocs")
}

func iCloudAvailable() bool { return exists(iCloudDrive()) }

// aCopyDir: kopia udziału A sejfu w iCloud Drive.
func aCopyDir(name string) string { return filepath.Join(iCloudDrive(), "xordock", name) }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

const adpWarning = "Kopia udziału A trafia do iCloud Drive. Konto iCloud + dowolna karta B = komplet: włącz 2FA i Advanced Data Protection, trzymaj karty osobno."

func warnings() {
	fmt.Println("Maski:", split.Kind(split.CSPRNG{}))
}

// subcommand zdejmuje pierwsze słowo bez myślnika (np. "create").
func subcommand(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
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

// findB szuka karty roboczej sejfu na podłączonych wolumenach po
// identyfikatorze sejfu, nie po nazwie wolumenu. Karty zapasowe pomija.
func findB(aRoot string) (string, error) {
	id, name, err := vault.ReadVaultID(aRoot)
	if err != nil {
		return "", fmt.Errorf("udział A: %w", err)
	}
	for _, c := range vault.FindCards("/Volumes", id) {
		if c.Role == "" {
			return c.Root, nil
		}
	}
	return "", fmt.Errorf("nie znaleziono karty roboczej sejfu %q (%s); podłącz właściwą kartę", name, id)
}

// spareRoot zwraca katalog sejfu na karcie zapasowej.
func spareRoot(volume, name string) (string, error) {
	if volume == "" {
		return "", errors.New("podaj --spare /Volumes/KARTA_ZAPASOWA")
	}
	if !strings.HasPrefix(filepath.Clean(volume), "/Volumes/") {
		return "", errors.New("--spare musi wskazywać wolumen w /Volumes")
	}
	return filepath.Join(volume, "xordock", name), nil
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
	if _, err := os.Stat(*to); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s już istnieje", *to)
	}
	v, err := open(c)
	if err != nil {
		return err
	}
	defer v.Close()
	fmt.Println("UWAGA: wyeksportowane pliki NIE są chronione podziałem XOR.")
	n, err := vault.ExportTo(v, *to)
	if err != nil {
		return err
	}
	fmt.Printf("Wyeksportowano %d plików do %s\n", n, *to)
	return nil
}
