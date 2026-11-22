// xordock: a protected macOS folder stored as two XOR shares,
// A on the Mac's SSD and B on an SD card. See specs/S004-xordock-macos-mvp.md.
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

const usage = `xordock — a folder in Finder, data split by XOR between the SSD and an SD card

Usage (common flags: --name VAULT, --a DIR_A):
  xordock init     --card /Volumes/CARD
  xordock mount    [--at /Volumes/xordock-<name>]   folder in Finder (macFUSE, FSKit backend)
  xordock unmount  [--at …]
  xordock status                                    card, snapshots, last backup, what could be lost
  xordock fsck
  xordock export   --to DIR                         plain files, NO longer protected
  xordock snapshot create | list | rm --id ID
  xordock backup   --spare /Volumes/SPARE [--snapshot ID] [--no-icloud]
                   B to the spare card + A to iCloud Drive (same point)
  xordock backup   prune --spare /Volumes/SPARE     keeps only the newest point (card and iCloud)
  xordock recover  --spare /Volumes/SPARE list | verify | export --to DIR |
                   restore --card /Volumes/NEW [--new-name NAME]   [--point ID]
  xordock recover  --from-icloud --spare /Volumes/CARD …   after losing the SSD: A from iCloud,
                   B from the spare or working card

Requirements: xordock doctor [--probe-mount] [--json]

Queries for the app (JSON):
  xordock vaults | volumes | points --spare /Volumes/SPARE

Share A by default: ~/Library/Application Support/xordock/<name>
Share B: <card>/xordock/<name>, recognized by the vault ID.
Backup requires the working card and the spare card connected at the same time.
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
	case "doctor":
		err = cmdDoctor(os.Args[2:])
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
	fl.Func("name", "vault name (default sejf)", func(s string) error {
		if err := validName(s); err != nil {
			return err
		}
		c.name = s
		return nil
	})
	fl.StringVar(&c.a, "a", "", "share A directory (default: in Application Support)")
}

func (c *common) aRoot() string {
	if c.a != "" {
		return c.a
	}
	return filepath.Join(home(), "Library", "Application Support", "xordock", c.name)
}

// validName: the vault name is a component of the A, B and mount point paths,
// so it cannot contain a separator or start with a dot (S007, the same
// rule as in the app).
func validName(s string) error {
	switch {
	case s == "":
		return errors.New("empty vault name")
	case len([]rune(s)) > 64:
		return errors.New("vault name longer than 64 characters")
	case strings.HasPrefix(s, "."):
		return errors.New("vault name cannot start with a dot")
	case strings.ContainsAny(s, "/:"):
		return errors.New("vault name cannot contain \"/\" or \":\"")
	case strings.TrimSpace(s) != s:
		return errors.New("vault name cannot start or end with a space")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return errors.New("vault name contains a control character")
		}
	}
	return nil
}

// iCloudDrive: the user's iCloud Drive directory (S008, fixed location).
func iCloudDrive() string {
	return filepath.Join(home(), "Library", "Mobile Documents", "com~apple~CloudDocs")
}

func iCloudAvailable() bool { return exists(iCloudDrive()) }

// aCopyDir: the vault's share A copy in iCloud Drive.
func aCopyDir(name string) string { return filepath.Join(iCloudDrive(), "xordock", name) }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

const adpWarning = "The share A copy goes to iCloud Drive. iCloud account + any B card = the full set: turn on 2FA and Advanced Data Protection, keep the cards separate."

func warnings() {
	fmt.Println("Masks:", split.Kind(split.CSPRNG{}))
}

// subcommand takes the first word without a dash (e.g. "create").
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
	card := fl.String("card", "", "SD card volume, e.g. /Volumes/xordock")
	fl.Parse(args)
	if *card == "" {
		return errors.New("--card is required")
	}
	if !strings.HasPrefix(filepath.Clean(*card), "/Volumes/") {
		return errors.New("--card must point to a volume in /Volumes")
	}
	bRoot := filepath.Join(*card, "xordock", c.name)
	v, err := vault.Create(c.aRoot(), bRoot, c.name, vault.Options{})
	if err != nil {
		return err
	}
	fmt.Printf("Created vault %q (%s)\n  A: %s\n  B: %s\n", c.name, v.ID(), c.aRoot(), bRoot)
	warnings()
	fmt.Println("Data cannot be restored without the card. Losing the card = losing the data (copy the folder from the card to a second card).")
	return nil
}

// findB looks for the vault's working card on connected volumes by
// vault ID, not by volume name. Skips spare cards.
func findB(aRoot string) (string, error) {
	id, name, err := vault.ReadVaultID(aRoot)
	if err != nil {
		return "", fmt.Errorf("share A: %w", err)
	}
	for _, c := range vault.FindCards("/Volumes", id) {
		if c.Role == "" {
			return c.Root, nil
		}
	}
	return "", fmt.Errorf("working card for vault %q (%s) not found; connect the right card", name, id)
}

// spareRoot returns the vault directory on the spare card.
func spareRoot(volume, name string) (string, error) {
	if volume == "" {
		return "", errors.New("--spare /Volumes/SPARE_CARD is required")
	}
	if !strings.HasPrefix(filepath.Clean(volume), "/Volumes/") {
		return "", errors.New("--spare must point to a volume in /Volumes")
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
		fmt.Println("Recovery:", v.Recovery)
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
	fmt.Printf("commit %d: %d files, %d directories, %d B of data, %d objects\n", r.Seq, r.Files, r.Dirs, r.Bytes, r.Objects)
	for _, p := range r.Problems {
		fmt.Println("ERROR:", p)
	}
	if !r.OK() {
		return fmt.Errorf("%d problems found", len(r.Problems))
	}
	fmt.Println("OK")
	return nil
}

func cmdExport(args []string) error {
	fl := flag.NewFlagSet("export", flag.ExitOnError)
	var c common
	c.bind(fl)
	to := fl.String("to", "", "target directory (must not exist)")
	fl.Parse(args)
	if *to == "" {
		return errors.New("--to is required")
	}
	if _, err := os.Stat(*to); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s already exists", *to)
	}
	v, err := open(c)
	if err != nil {
		return err
	}
	defer v.Close()
	fmt.Println("WARNING: exported files are NOT protected by the XOR split.")
	n, err := vault.ExportTo(v, *to)
	if err != nil {
		return err
	}
	fmt.Printf("Exported %d files to %s\n", n, *to)
	return nil
}
