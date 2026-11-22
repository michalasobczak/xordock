package vault

import (
	"path/filepath"
)

// Card is a vault directory found on a volume (working or spare card).
type Card struct {
	Root  string
	Role  string // "" = working, "backup" = spare
	Media string
}

// FindCards searches <volumes>/*/xordock/*/ and returns the directories belonging to
// vault id. We recognize them by the vault ID from the header, not by the volume
// or directory name.
func FindCards(volumes string, id ID) []Card {
	var out []Card
	matches, _ := filepath.Glob(filepath.Join(volumes, "*", "xordock", "*", headerFile))
	for _, m := range matches {
		root := filepath.Dir(m)
		h, err := readHeader(root, SideB)
		if err != nil || h.Vault != id.String() {
			continue
		}
		out = append(out, Card{Root: root, Role: h.Role, Media: h.Media})
	}
	return out
}

// CardEntry is a vault directory on a volume with the ID and name
// from its header (the device list in the app, S007 U-02).
type CardEntry struct {
	Card
	Vault string
	Name  string
}

// ListCards returns the vault directories (working and spare) on one volume.
func ListCards(volume string) []CardEntry {
	var out []CardEntry
	matches, _ := filepath.Glob(filepath.Join(volume, "xordock", "*", headerFile))
	for _, m := range matches {
		root := filepath.Dir(m)
		h, err := readHeader(root, SideB)
		if err != nil {
			continue
		}
		out = append(out, CardEntry{Card: Card{Root: root, Role: h.Role, Media: h.Media}, Vault: h.Vault, Name: h.Name})
	}
	return out
}
