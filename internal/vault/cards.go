package vault

import (
	"path/filepath"
)

// Card to katalog sejfu znaleziony na wolumenie (karta robocza albo zapasowa).
type Card struct {
	Root  string
	Role  string // "" = robocza, "backup" = zapasowa
	Media string
}

// FindCards przeszukuje <volumes>/*/xordock/*/ i zwraca katalogi należące do
// sejfu id. Rozpoznajemy po identyfikatorze sejfu z nagłówka, nie po nazwie
// wolumenu ani katalogu.
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

// CardEntry to katalog sejfu na wolumenie wraz z identyfikatorem i nazwą
// z nagłówka (lista nośników w aplikacji, S007 U-02).
type CardEntry struct {
	Card
	Vault string
	Name  string
}

// ListCards zwraca katalogi sejfów (roboczych i zapasowych) na jednym wolumenie.
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
