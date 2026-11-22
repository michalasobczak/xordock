# S006 — Udziały A i B na różnych fizycznych nośnikach

- **Status:** szkic (decyzje właściciela zebrane 2026-11-22; implementacja po S007)
- **Data:** 2026-11-22
- **Powiązane:** S004 (D-01, B-03), S005, S007

## 1. TL;DR

xordock ma odmawiać pracy, gdy udział A, karta robocza B albo karta zapasowa leżą na tym samym **fizycznym** dysku. Dziś porównywany jest tylko numer woluminu (`st_dev`). Wynik potwierdzą testy z podstawionym rozpoznawaniem dysku i test na obrazie dysku leżącym na SSD.

## 2. Abstract

`vault.Create` i `prepareSpare` porównują `st_dev` ścieżek. Każdy wolumin ma własny numer, więc kontrolę przechodzą:

- obraz `.dmg` albo sparsebundle leżący na SSD obok A. Potwierdzone 2026-11-22: `init` przyjął B na obrazie z `scratchpad`;
- druga partycja albo drugi wolumin APFS na tym samym dysku;
- drugi wolumin na tym samym pendrivie, użyty jako karta zapasowa.

Do tego wynik sprawdzenia jest ignorowany (`same, _ := sameDevice(...)`). Gdy `MkdirAll` w `/Volumes` się nie uda, kontrola przepuszcza (fail-open), a `Create` zdąży zapisać nagłówek A, zanim zapis B się wyłoży. Zdarzyło się to właścicielowi przy literówce w nazwie woluminu: został osierocony `xordock.json` w A. Ta sama literówka uruchomiona z `sudo` utworzyła katalog `/Volumes/<literówka>` na dysku systemowym. Uruchamianie jako root zostawia też pliki A należące do roota.

Kontrola działa tylko przy `init` i backupie. B przeniesione później na SSD (skopiowany katalog) otwiera się bez sprzeciwu.

## 3. Objective

- **Cele (decyzje właściciela 2026-11-22):**
  - **Kiedy:** zawsze. Przy `init`, przy każdym otwarciu sejfu (mount, status, fsck, snapshot, export, backup), przy backupie (zapasowa ≠ dysk A i ≠ dysk B) i przy `recover` (zapasowa ≠ dysk A, nowa karta ≠ dysk A).
  - **Obrazy dysku:** wyznaczamy fizyczny dysk, na którym leży plik obrazu, i porównujemy go jak zwykły nośnik.
  - **Nośnik nieustalony** (wolumin sieciowy, iCloud, błąd `diskutil`): odmowa (fail-closed).
  - **Obejście:** brak. Kontrolę wyłączają tylko testy jednostkowe w kodzie `_test.go`.
  - Brak skutków ubocznych przy odmowie: żadnych katalogów w `/Volumes` na dysku systemowym, żadnego nagłówka A.
  - Propozycja do potwierdzenia: odmowa pracy jako root (`euid == 0`).
- **Poza zakresem:** wykrywanie, że dwa różne fizyczne dyski są w jednej obudowie lub RAID.
- **Kryteria sukcesu:** bramki z §7 przechodzą.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `internal/vault/media.go` | kod: fizyczny dysk ścieżki (statfs → diskutil → APFS physical store → hdiutil image-path) | do utworzenia |
| `internal/vault/vault.go`, `backup.go`, `recovery.go` | kod | do zmiany |
| `cmd/xordock/main.go` | kod: `--card` musi być punktem montowania | do zmiany |

## 5. Functional Domain Changes

[DO USTALENIA przy implementacji. Zasada: identyfikator fizycznego dysku = whole disk po rozwinięciu APFS physical store i plików obrazów, z ograniczoną głębokością rekurencji.]

## 6. Critical Path

[DO USTALENIA]

## 7. Control Gates

| # | Bramka | Warunek | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | B na obrazie na SSD | odmowa | test darwin z `hdiutil` | commit |
| G2 | Błąd rozpoznania | odmowa, nic nie zapisane | test z podstawionym resolverem | commit |
| G3 | Open sprawdza A ≠ B | odmowa | test | commit |
| G4 | Literówka w `--card` | odmowa bez tworzenia katalogu w `/Volumes` | test CLI | commit |

## 8. Expected Outcome

Nie da się, nawet przez pomyłkę, trzymać A i B na jednym fizycznym nośniku.

## 9. Additional Information

Kolejność: właściciel zdecydował, że najpierw powstaje aplikacja (S007).
