# S011 — Sprawdzanie wymagań przy starcie

- **Status:** dostarczona
- **Data:** 2026-11-22
- **Powiązane:** S004 (A-12, ADR-003), S007, S008, S010

## 1. TL;DR

Przy starcie aplikacja sprawdza, czy są wszystkie narzędzia:

- macOS 26+;
- macFUSE 5.5+;
- włączone rozszerzenie systemu plików macFUSE;
- działający silnik `xordock`;
- FileVault i iCloud Drive jako zalecane.

Jeśli czegoś brakuje, pokazuje listę z przyciskiem naprawy: pobierz macFUSE albo otwórz Ustawienia systemowe. Gdy wszystko jest w porządku, nic nie pokazuje.

## 2. Abstract

Do działania xordocka potrzebne jest oprogramowanie, którego aplikacja nie dołącza (S004 A-12: licencja macFUSE). Właściciel przy pierwszym uruchomieniu trafił na błąd „File system extension not enabled”, który wyszedł dopiero przy montowaniu, a CLI wypisało wtedy mylący sukces (naprawione w S004).

Dodatkowo CLI było linkowane z `libfuse3` na sztywno. Bez macFUSE nie uruchomiłoby się wcale, także do poleceń, które macFUSE nie potrzebują (status, backup, odzyskiwanie).

Właściciel poprosił (2026-11-22) o sprawdzanie przy uruchomieniu, czy wszystkie narzędzia są. Decyzje wykonawcze (domyślne, do potwierdzenia):

- **Wymagane (czerwone):** macOS 26+ (minimum modułu FSKit macFUSE: `LSMinimumSystemVersion` 26.0), macFUSE ≥ 5.5, rozszerzenie włączone, CLI działa.
- **Zalecane (pomarańczowe):** FileVault, iCloud Drive (kopia A, S008).
- **Ekran wymagań:** pokazuje się przy starcie tylko wtedy, gdy coś wymaganego nie działa. Jest też dostępny z Ustawień i z menu kłódki. Wyprzedza pytanie o samouczek.
- **Rozszerzenie FSKit:** jego stan leży w chronionym kontenerze systemu, którego aplikacja nie może czytać. Sprawdzamy go **próbnym montowaniem** pustego wolumenu tylko do odczytu, po czym od razu odmontowujemy. Wynik zapamiętujemy dla danej wersji macFUSE, a każde udane montowanie sejfu też go potwierdza, więc próba nie powtarza się przy każdym starcie.

## 3. Objective

- **Cele:**
  - W-01 `xordock doctor [--probe-mount] [--json]`: lista sprawdzeń `{id, title, level: ok|warning|error|unknown, detail, fixLabel, fixURL}`.
  - W-02 CLI linkuje `libfuse3` słabo. Bez macFUSE uruchamia się, `doctor` zgłasza brak, a `mount` odmawia z jasnym komunikatem zamiast awarii loadera.
  - W-03 Próbne montowanie: system plików bez sejfu (stan „niedostępny”: katalog główny pusty, każda inna operacja EIO), montowany w katalogu tymczasowym na najwyżej 15 s.
  - W-04 Aplikacja przy starcie uruchamia `doctor --json`. Próbę montowania dokłada, gdy wymagane rzeczy są w porządku, a stan rozszerzenia nie jest potwierdzony dla bieżącej wersji macFUSE. Arkusz „Wymagania” ma przyciski naprawy i „Sprawdź ponownie”.
- **Poza zakresem:** instalowanie macFUSE przez aplikację, sprawdzanie kart (to robi pulpit).
- **Kryteria sukcesu:** bramki z §7.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `cmd/xordock/doctor.go` | kod | do utworzenia |
| `internal/fusemount/probe.go`, `bridge*` | kod: próba montowania, słabe linkowanie, `Available()` | do utworzenia / do zmiany |
| `internal/fusefs/fs.go` | kod: `NewProbe` | do zmiany |
| `macos/Sources/XordockCore/Requirements.swift`, `XordockUI`/`XordockApp` | kod | do utworzenia / do zmiany |

## 5. Functional Domain Changes

- Próbny wolumen nie ma dostępu do żadnego sejfu ani karty. Ma nazwę `xordock-sprawdzenie`, a jego punkt montowania jest katalogiem tymczasowym, który potem jest usuwany.
- Brak macFUSE nie blokuje poleceń bez montowania (odzyskiwanie po awarii nadal działa).
- Wynik próby: „ok”, „error” (rozszerzenie wyłączone albo montowanie nieudane) albo „unknown” (bez próby).

## 6. Critical Path

1. `NewProbe` i `Probe` z testem.
2. Słabe linkowanie z `Available()`.
3. `doctor`.
4. Aplikacja.
5. Próba na prawdziwym macFUSE.

## 7. Control Gates

| # | Bramka | Warunek | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | Próbny FS | katalog główny odpowiada, każda inna operacja EIO, żadna nie panikuje bez sejfu | `go test` | commit |
| G2 | Próba na sprzęcie | `doctor --probe-mount` daje „ok” przy włączonym rozszerzeniu; wolumen znika, katalog tymczasowy usunięty | E2E | commit |
| G3 | Brak macFUSE | CLI z `DYLD_LIBRARY_PATH` bez libfuse3 i podmienioną ścieżką: `doctor` działa, `mount` odmawia z komunikatem | test ręczny | commit |
| G4 | Aplikacja | dekodowanie wyniku, reguła „pokaż arkusz przy starcie tylko przy błędzie”, „próba, gdy niepotwierdzone” | `swift test` | commit |
| G5 | **Negatywna** | próba nie dotyka katalogów sejfów ani kart | przegląd + test G1 | commit |

## 8. Expected Outcome

Brakujące narzędzie widać od razu przy starcie, z przyciskiem naprawy, zamiast błędu przy pierwszym montowaniu.

## 9. Additional Information

- Odrzucone: czytanie stanu FSKit z kontenerów systemu (chronione, „Operation not permitted”) i `pluginkit` (moduł macFUSE nie jest tam widoczny).
- Pobieranie macFUSE: oficjalna strona `https://macfuse.github.io/`. Aplikacja tylko ją otwiera.

## Realizacja (2026-11-22)

- **Słabe linkowanie:**
  - Flaga `-weak-lfuse3` jest odrzucana przez listę dozwolonych flag cgo.
  - Zamiast niej `ops_darwin.c` ładuje `/usr/local/lib/libfuse3.4.dylib` przez `dlopen` przy pierwszym montowaniu. Ścieżka jest stała, bez przeszukiwania `DYLD_*`.
  - Używanych jest 6 symboli; `fuse_new` to wrapper inline, więc wołamy bezpośrednio `_fuse_new_31` ze strukturą wersji jak w nagłówku.
  - `otool -L bin/xordock` nie pokazuje już libfuse.
  - `Mount` i `Probe` zwracają `ErrNoMacFUSE`, gdy biblioteki brak.
- **Wolumen próbny:**
  - `fusefs.NewProbe()` (bez sejfu, stan „niedostępny”), `FS.IsProbe()` (bez logu zatwierdzania przy odmontowaniu);
  - `fusemount.Probe` montuje w `os.MkdirTemp`, czeka na wolumen do 15 s, odmontowuje i usuwa katalog. Na macOS 27 z macFUSE 5.5.0 trwa ok. 1 s.
- **`xordock doctor`:**
  - sprawdza macOS (≥ 26), macFUSE (plist + `dlopen`, ≥ 5.5), rozszerzenie (próba tylko z `--probe-mount`), FileVault, iCloud Drive;
  - tryb tekstowy kończy się kodem ≠ 0 przy brakach wymaganych;
  - stałe adresy naprawy: strona macFUSE i panele Ustawień systemowych.
- **Aplikacja:**
  - przy starcie `model.startup()`: `doctor`, a gdy rozszerzenie niepotwierdzone dla bieżącej wersji macFUSE, `doctor --probe-mount`;
  - wynik zapamiętany w `UserDefaults fskitConfirmedForMacFUSE`; każde udane montowanie sejfu go potwierdza;
  - arkusz „Wymagania” pokazuje się przy starcie tylko przy brakach i wyprzedza pytanie o samouczek;
  - dostęp: menu kłódki „Wymagania…” i Ustawienia „Sprawdź wymagania” (z próbą).
- **Testy:**
  - Go: `TestProbeFS` (G1, G5), `TestDoctor` (G3: brak macFUSE, stara wersja, wyłączone rozszerzenie, stary macOS, porównanie wersji);
  - Swift: `requirementsRules` (G4), razem 17.
- **E2E (G2):**
  - `doctor --probe-mount`: „Próbne montowanie udane”, brak wolumenu i katalogu tymczasowego po próbie;
  - sejf tymczasowy na `xor_primary`: montowanie, zapis i odczyt przez Findera z biblioteką ładowaną przez `dlopen`, odmontowanie; sejf usunięty;
  - aplikacja przy starcie zapisała potwierdzenie dla 5.5.0.
- **Rendery:** `docs/ui/wymagania-jasny.png`.
