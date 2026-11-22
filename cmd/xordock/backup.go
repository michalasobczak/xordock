package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"xordock/internal/vault"
)

type Point = vault.Point

// Polecenia migawek, backupu, odzyskiwania i stanu (S004 §5.4–5.6, G-08).

func cmdStatus(args []string) error {
	fl := flag.NewFlagSet("status", flag.ExitOnError)
	var c common
	c.bind(fl)
	fl.Parse(args)
	id, name, err := vault.ReadVaultID(c.aRoot())
	if err != nil {
		return fmt.Errorf("udział A: %w", err)
	}
	fmt.Printf("Sejf %q (%s)\n  A: %s\n", name, id, c.aRoot())
	cards := vault.FindCards("/Volumes", id)
	for _, k := range cards {
		role := "robocza"
		if k.Role != "" {
			role = "zapasowa"
		}
		fmt.Printf("  karta %s: %s\n", role, k.Root)
	}
	b, err := findB(c.aRoot())
	if err != nil {
		fmt.Println("  Karta robocza: BRAK. Sejf niedostępny. Po utracie karty: xordock recover --spare …")
		return nil
	}
	v, err := vault.Open(c.aRoot(), b, vault.Options{})
	if err != nil {
		return err
	}
	defer v.Close()
	fmt.Printf("  commit %d, migawek: %d\n  Maski: %s\n", v.Seq(), len(v.Snapshots()), v.MaskKind())
	if jc, ok := readCopy(aCopyDir(c.name)); ok && jc.ID == id.String() && jc.LastPoint != "" {
		fmt.Printf("  Kopia udziału A w iCloud: %s, ostatni punkt %s\n", jc.Path, jc.LastPoint)
	} else {
		fmt.Println("  Kopia udziału A w iCloud: BRAK. Utrata SSD = utrata danych.")
	}
	ref, snap, ok := v.LastBackup()
	if !ok {
		fmt.Println("  Backup: BRAK potwierdzonego punktu odzyskiwania. Utrata karty roboczej = utrata danych.")
		return nil
	}
	fmt.Printf("  Ostatni backup READY: %s, punkt %s, migawka %s (commit %d), nośnik %s\n",
		ref.Time.Local().Format("2006-01-02 15:04"), ref.Point, snap.ID, snap.Seq, ref.Media)
	ch, err := v.Changes(snap.ID)
	if err != nil {
		return err
	}
	if len(ch) == 0 {
		fmt.Println("  Od backupu bez zmian.")
	} else {
		fmt.Printf("  Po utracie karty roboczej przepadnie %d zmian (ścieżek) od tego backupu.\n", len(ch))
	}
	return nil
}

func cmdSnapshot(args []string) error {
	sub, args := subcommand(args)
	fl := flag.NewFlagSet("snapshot", flag.ExitOnError)
	var c common
	c.bind(fl)
	sid := fl.String("id", "", "identyfikator migawki")
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
		fmt.Printf("Migawka %s (commit %d)\n", s.ID, s.Seq)
	case "list", "":
		for _, s := range v.Snapshots() {
			ready := 0
			for _, p := range s.Points {
				if p.Ready {
					ready++
				}
			}
			fmt.Printf("%s  commit %-5d %s  punktów odzyskiwania: %d\n", s.ID, s.Seq, s.Time.Local().Format("2006-01-02 15:04"), ready)
		}
	case "rm":
		if *sid == "" {
			return errors.New("podaj --id")
		}
		if err := v.DeleteSnapshot(*sid); err != nil {
			return err
		}
		fmt.Println("Usunięto migawkę", *sid)
	default:
		return fmt.Errorf("nieznane polecenie snapshot %q", sub)
	}
	return nil
}

func cmdBackup(args []string) error {
	sub, args := subcommand(args)
	fl := flag.NewFlagSet("backup", flag.ExitOnError)
	var c common
	c.bind(fl)
	spareVol := fl.String("spare", "", "wolumen karty zapasowej, np. /Volumes/zapas")
	sid := fl.String("snapshot", "", "migawka (domyślnie najnowsza)")
	noICloud := fl.Bool("no-icloud", false, "nie kopiuj udziału A do iCloud Drive (S008)")
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
			fmt.Printf("Punkt odzyskiwania READY: %s\n  migawka %s (commit %d), %d obiektów, %d B\n  utworzony %s\n  udziały B: %s\n",
				p.ID, p.Snapshot, p.Seq, p.Objects, p.Bytes, p.Created.Local().Format("2006-01-02 15:04"), spare)
		}
		if pa.ID != "" {
			fmt.Printf("  udziały A: %s (iCloud Drive)\n", aCopy)
		}
		if err != nil {
			return err
		}
		switch {
		case *noICloud:
			fmt.Println("UWAGA: kopia udziału A pominięta (--no-icloud). Utrata SSD = utrata danych, o ile nie masz innej kopii A.")
		case !useICloud:
			fmt.Println("UWAGA: iCloud Drive niedostępny, kopia udziału A NIE powstała. Utrata SSD = utrata danych.")
		}
		fmt.Println("UWAGA: zmiany wykonane po tej migawce NIE znajdują się w kopiach (B-05).")
	case "prune":
		np, no, err := v.PruneBackup(spare)
		if err != nil {
			return err
		}
		fmt.Printf("Karta zapasowa: usunięto %d starszych punktów i %d udziałów; najnowszy punkt zostaje.\n", np, no)
		if aCopy := aCopyDir(c.name); exists(aCopy) {
			np, no, err := v.PruneACopy(aCopy)
			if err != nil {
				return fmt.Errorf("kopia A w iCloud: %w", err)
			}
			fmt.Printf("Kopia A w iCloud: usunięto %d starszych punktów i %d udziałów; najnowszy punkt zostaje.\n", np, no)
		}
	default:
		return fmt.Errorf("nieznane polecenie backup %q", sub)
	}
	return nil
}

func cmdRecover(args []string) error {
	fl := flag.NewFlagSet("recover", flag.ExitOnError)
	var c common
	c.bind(fl)
	spareVol := fl.String("spare", "", "wolumen karty zapasowej")
	point := fl.String("point", "", "punkt odzyskiwania (domyślnie najnowszy READY)")
	to := fl.String("to", "", "export: katalog docelowy")
	card := fl.String("card", "", "restore: wolumen nowej karty roboczej")
	newName := fl.String("new-name", "", "restore: nazwa nowego sejfu (domyślnie <name>-odzyskany)")
	newA := fl.String("new-a", "", "restore: katalog nowego udziału A")
	fromICloud := fl.Bool("from-icloud", false, "udział A z kopii w iCloud Drive (po utracie SSD, S008)")
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
			fmt.Printf("%s  READY  commit %-5d %s  %d obiektów\n", p.ID, p.Seq, p.Created.Local().Format("2006-01-02 15:04"), p.Objects)
		}
		for _, b := range bad {
			fmt.Println("odrzucony:", b)
		}
		if len(pts) == 0 {
			return vault.ErrNoPoint
		}
	case "verify":
		p, n, err := r.Verify(*point)
		if err != nil {
			return err
		}
		fmt.Printf("Punkt %s kompletny: %d plików złożonych i sprawdzonych.\n", p.ID, n)
	case "export":
		if *to == "" {
			return errors.New("podaj --to")
		}
		w, p, err := r.View(*point)
		if err != nil {
			return err
		}
		fmt.Println("UWAGA: wyeksportowane pliki NIE są chronione podziałem XOR.")
		n, err := vault.ExportTo(w, *to)
		if err != nil {
			return err
		}
		fmt.Printf("Wyeksportowano %d plików z punktu %s do %s\n", n, p.ID, *to)
	case "restore":
		if *card == "" {
			return errors.New("podaj --card /Volumes/NOWA_KARTA")
		}
		name := *newName
		if name == "" {
			name = c.name + "-odzyskany"
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
			return fmt.Errorf("odtwarzanie przerwane po %d plikach: %w", n, err)
		}
		fmt.Printf("Odtworzono %d plików z punktu %s do nowego sejfu %q (nowe maski)\n  A: %s\n  B: %s\n", n, p.ID, name, aDir, bDir)
		fmt.Println("Karta zapasowa i stary udział A są nietknięte. Zgubioną kartę traktuj jako skopiowaną;")
		fmt.Println("po sprawdzeniu nowego sejfu usuń stary udział A, żeby zgubiona karta niczego już nie odsłaniała.")
	default:
		return fmt.Errorf("nieznane polecenie recover %q", sub)
	}
	return nil
}
