package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"

	"xordock/internal/vault"
)

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
		p, err := v.Backup(spare, *sid)
		if err != nil {
			return err
		}
		fmt.Printf("Punkt odzyskiwania READY: %s\n  migawka %s (commit %d), %d obiektów, %d B\n  utworzony %s\n",
			p.ID, p.Snapshot, p.Seq, p.Objects, p.Bytes, p.Created.Local().Format("2006-01-02 15:04"))
		fmt.Println("UWAGA: zmiany wykonane po tej migawce NIE znajdują się na karcie zapasowej (B-05).")
	case "prune":
		np, no, err := v.PruneBackup(spare)
		if err != nil {
			return err
		}
		fmt.Printf("Usunięto %d starszych punktów i %d udziałów z karty zapasowej; najnowszy punkt zostaje.\n", np, no)
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
	fl.Parse(args)
	sub, rest := subcommand(fl.Args())
	if len(rest) > 0 {
		fl.Parse(rest)
	}
	spare, err := spareRoot(*spareVol, c.name)
	if err != nil {
		return err
	}
	r, err := vault.OpenRecovery(c.aRoot(), spare)
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
