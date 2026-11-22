package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"xordock/internal/vault"
)

type Point = vault.Point

// Snapshot, backup, recovery and status commands (S004 §5.4–5.6, G-08).

func cmdStatus(args []string) error {
	fl := flag.NewFlagSet("status", flag.ExitOnError)
	var c common
	c.bind(fl)
	fl.Parse(args)
	id, name, err := vault.ReadVaultID(c.aRoot())
	if err != nil {
		return fmt.Errorf("share A: %w", err)
	}
	fmt.Printf("Vault %q (%s)\n  A: %s\n", name, id, c.aRoot())
	cards := vault.FindCards("/Volumes", id)
	for _, k := range cards {
		role := "working"
		if k.Role != "" {
			role = "spare"
		}
		fmt.Printf("  %s card: %s\n", role, k.Root)
	}
	b, err := findB(c.aRoot())
	if err != nil {
		fmt.Println("  Working card: MISSING. Vault unavailable. If the card is lost: xordock recover --spare …")
		return nil
	}
	v, err := vault.Open(c.aRoot(), b, vault.Options{})
	if err != nil {
		return err
	}
	defer v.Close()
	fmt.Printf("  commit %d, snapshots: %d\n  Masks: %s\n", v.Seq(), len(v.Snapshots()), v.MaskKind())
	if jc, ok := readCopy(aCopyDir(c.name)); ok && jc.ID == id.String() && jc.LastPoint != "" {
		fmt.Printf("  Share A copy in iCloud Drive: %s, last point %s\n", jc.Path, jc.LastPoint)
	} else {
		fmt.Println("  Share A copy in iCloud Drive: MISSING. Losing the SSD = losing the data.")
	}
	ref, snap, ok := v.LastBackup()
	if !ok {
		fmt.Println("  Backup: NO confirmed recovery point. Losing the working card = losing the data.")
		return nil
	}
	fmt.Printf("  Last READY backup: %s, point %s, snapshot %s (commit %d), medium %s\n",
		ref.Time.Local().Format("2006-01-02 15:04"), ref.Point, snap.ID, snap.Seq, ref.Media)
	ch, err := v.Changes(snap.ID)
	if err != nil {
		return err
	}
	if len(ch) == 0 {
		fmt.Println("  No changes since the backup.")
	} else {
		fmt.Printf("  Losing the working card would lose %d changes (paths) made since this backup.\n", len(ch))
	}
	return nil
}

func cmdSnapshot(args []string) error {
	sub, args := subcommand(args)
	fl := flag.NewFlagSet("snapshot", flag.ExitOnError)
	var c common
	c.bind(fl)
	sid := fl.String("id", "", "snapshot ID")
	fl.Parse(args)
	v, err := open(c)
	if err != nil {
		return err
	}
	defer v.Close()
	switch sub {
	case "create":
		s, err := v.CreateSnapshot()
		if err != nil {
			return err
		}
		fmt.Printf("Snapshot %s (commit %d)\n", s.ID, s.Seq)
	case "list", "":
		for _, s := range v.Snapshots() {
			ready := 0
			for _, p := range s.Points {
				if p.Ready {
					ready++
				}
			}
			fmt.Printf("%s  commit %-5d %s  recovery points: %d\n", s.ID, s.Seq, s.Time.Local().Format("2006-01-02 15:04"), ready)
		}
	case "rm":
		if *sid == "" {
			return errors.New("--id is required")
		}
		if err := v.DeleteSnapshot(*sid); err != nil {
			return err
		}
		fmt.Println("Deleted snapshot", *sid)
	default:
		return fmt.Errorf("unknown snapshot command %q", sub)
	}
	return nil
}

func cmdBackup(args []string) error {
	sub, args := subcommand(args)
	fl := flag.NewFlagSet("backup", flag.ExitOnError)
	var c common
	c.bind(fl)
	spareVol := fl.String("spare", "", "spare card volume, e.g. /Volumes/spare")
	sid := fl.String("snapshot", "", "snapshot (default: the newest)")
	noICloud := fl.Bool("no-icloud", false, "do not copy share A to iCloud Drive (S008)")
	fl.Parse(args)
	spare, err := spareRoot(*spareVol, c.name)
	if err != nil {
		return err
	}
	v, err := open(c)
	if err != nil {
		return err
	}
	defer v.Close()
	switch sub {
	case "":
		aCopy, useICloud := aCopyDir(c.name), !*noICloud && iCloudAvailable()
		var p, pa Point
		if useICloud {
			fmt.Println(adpWarning)
			p, pa, err = v.BackupPair(spare, aCopy, *sid)
		} else {
			p, err = v.Backup(spare, *sid)
		}
		if p.ID != "" {
			fmt.Printf("READY recovery point: %s\n  snapshot %s (commit %d), %d objects, %d B\n  created %s\n  B shares: %s\n",
				p.ID, p.Snapshot, p.Seq, p.Objects, p.Bytes, p.Created.Local().Format("2006-01-02 15:04"), spare)
		}
		if pa.ID != "" {
			fmt.Printf("  A shares: %s (iCloud Drive)\n", aCopy)
		}
		if err != nil {
			return err
		}
		switch {
		case *noICloud:
			fmt.Println("WARNING: share A copy skipped (--no-icloud). Losing the SSD = losing the data, unless you have another copy of A.")
		case !useICloud:
			fmt.Println("WARNING: iCloud Drive unavailable, the share A copy was NOT created. Losing the SSD = losing the data.")
		}
		fmt.Println("WARNING: changes made after this snapshot are NOT in the backups (B-05).")
	case "prune":
		np, no, err := v.PruneBackup(spare)
		if err != nil {
			return err
		}
		fmt.Printf("Spare card: deleted %d older points and %d shares; the newest point is kept.\n", np, no)
		if aCopy := aCopyDir(c.name); exists(aCopy) {
			np, no, err := v.PruneACopy(aCopy)
			if err != nil {
				return fmt.Errorf("share A copy in iCloud Drive: %w", err)
			}
			fmt.Printf("Share A copy in iCloud Drive: deleted %d older points and %d shares; the newest point is kept.\n", np, no)
		}
	default:
		return fmt.Errorf("unknown backup command %q", sub)
	}
	return nil
}

func cmdRecover(args []string) error {
	fl := flag.NewFlagSet("recover", flag.ExitOnError)
	var c common
	c.bind(fl)
	spareVol := fl.String("spare", "", "spare card volume")
	point := fl.String("point", "", "recovery point (default: the newest READY)")
	to := fl.String("to", "", "export: target directory")
	card := fl.String("card", "", "restore: volume of the new working card")
	newName := fl.String("new-name", "", "restore: name of the new vault (default <name>-recovered)")
	newA := fl.String("new-a", "", "restore: directory of the new share A")
	fromICloud := fl.Bool("from-icloud", false, "share A from the copy in iCloud Drive (after losing the SSD, S008)")
	fl.Parse(args)
	sub, rest := subcommand(fl.Args())
	if len(rest) > 0 {
		fl.Parse(rest)
	}
	spare, err := spareRoot(*spareVol, c.name)
	if err != nil {
		return err
	}
	aSrc := c.aRoot()
	if *fromICloud {
		aSrc = aCopyDir(c.name)
	}
	r, err := vault.OpenRecovery(aSrc, spare)
	if err != nil {
		return err
	}
	defer r.Close()
	switch sub {
	case "list", "":
		pts, bad := r.Points()
		for _, p := range pts {
			fmt.Printf("%s  READY  commit %-5d %s  %d objects\n", p.ID, p.Seq, p.Created.Local().Format("2006-01-02 15:04"), p.Objects)
		}
		for _, b := range bad {
			fmt.Println("rejected:", b)
		}
		if len(pts) == 0 {
			return vault.ErrNoPoint
		}
	case "verify":
		p, n, err := r.Verify(*point)
		if err != nil {
			return err
		}
		fmt.Printf("Point %s complete: %d files reassembled and verified.\n", p.ID, n)
	case "export":
		if *to == "" {
			return errors.New("--to is required")
		}
		w, p, err := r.View(*point)
		if err != nil {
			return err
		}
		fmt.Println("WARNING: exported files are NOT protected by the XOR split.")
		n, err := vault.ExportTo(w, *to)
		if err != nil {
			return err
		}
		fmt.Printf("Exported %d files from point %s to %s\n", n, p.ID, *to)
	case "restore":
		if *card == "" {
			return errors.New("--card /Volumes/NEW_CARD is required")
		}
		name := *newName
		if name == "" {
			name = c.name + "-recovered"
		}
		if err := validName(name); err != nil {
			return err
		}
		w, p, err := r.View(*point)
		if err != nil {
			return err
		}
		aDir := *newA
		if aDir == "" {
			aDir = filepath.Join(home(), "Library", "Application Support", "xordock", name)
		}
		bDir := filepath.Join(*card, "xordock", name)
		nv, err := vault.Create(aDir, bDir, name, vault.Options{})
		if err != nil {
			return err
		}
		defer nv.Close()
		n, err := vault.CopyInto(w, nv)
		if err != nil {
			return fmt.Errorf("restore interrupted after %d files: %w", n, err)
		}
		fmt.Printf("Restored %d files from point %s to the new vault %q (new masks)\n  A: %s\n  B: %s\n", n, p.ID, name, aDir, bDir)
		fmt.Println("The spare card and the old share A are untouched. Treat the lost card as copied;")
		fmt.Println("after checking the new vault, delete EVERY copy of the old share A (SSD, iCloud Drive including")
		fmt.Println("Recently Deleted, Time Machine) so the lost card no longer reveals anything (docs/recovery.md).")
	default:
		return fmt.Errorf("unknown recover command %q", sub)
	}
	return nil
}
