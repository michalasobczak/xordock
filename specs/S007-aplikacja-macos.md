# S007 — Aplikacja macOS: pasek menu i okno

- **Status:** dostarczona (G1–G5, G7, G8); G6 i G9 czekają na test właściciela na pendrive'ach
- **Data:** 2026-11-22
- **Powiązane:** S004 (CP-08, ADR-002, CG-14, §7), S006

## 1. TL;DR

Aplikacja SwiftUI z ikoną w pasku menu i oknem zastępuje ręczne wpisywanie poleceń CLI. Umie:

- zakładać sejf z wyborem nośnika z listy;
- montować i odmontowywać;
- pokazywać stan kart i aktualność backupu;
- robić migawkę z backupem jednym przyciskiem;
- odzyskiwać dane z karty zapasowej.

Silnik zostaje jeden: aplikacja uruchamia dołączony `xordock`.

## 2. Abstract

Po S004 xordock jest narzędziem CLI. Właściciel testował je na dwóch pendrive'ach i trafił na typowe pułapki:

- literówka w nazwie woluminu;
- `sudo`, które zostawiło pliki A należące do roota;
- mylące `snapshot` bez `create`;
- brak informacji, gdzie leżą udziały.

Wniosek: „trudno się w to klika”. S004 przewidywała UI jako CP-08 (ADR-002: SwiftUI do zarządzania wolumenem i nośnikami).

Decyzje właściciela (2026-11-22):

- **Forma:** pasek menu + okno.
- **Silnik:** aplikacja wywołuje CLI, nie ładuje Go jako biblioteki. Proces montowania żyje osobno, więc awaria lub zamknięcie UI nie zrywa montowania.
- **Zakres pierwszej wersji:** montowanie i stan, zakładanie sejfu, migawki z backupem, odzyskiwanie.
- **Kolejność:** UI przed S006.

## 3. Objective

- **Cele:**
  - U-01 lista sejfów z `~/Library/Application Support/xordock/*` ze stanem: karta robocza podłączona lub nie, zamontowany, karty zapasowe, ostatni backup READY i liczba zmian od niego;
  - U-02 zakładanie sejfu: nazwa i wolumin z listy podłączonych nośników zewnętrznych, bez wpisywania ścieżek;
  - U-03 mount i unmount, „Pokaż w Finderze”;
  - U-04 „Backup teraz” = odmontuj (za zgodą) → migawka → backup na wskazaną lub wykrytą kartę zapasową;
  - U-05 przypomnienie w pasku menu, gdy nie ma punktu READY albo ostatni ma więcej niż 7 dni (S004 B-01);
  - U-06 odzyskiwanie: lista punktów, weryfikacja, eksport do wybranego katalogu, odtworzenie na nową kartę;
  - U-07 czytelne błędy (tekst z CLI) i log procesu montowania.
- **Poza zakresem:**
  - powiadomienia systemowe;
  - automatyczny backup w tle;
  - start przy logowaniu;
  - podpis Developer ID i notaryzacja (aplikacja jest budowana lokalnie i podpisana ad hoc);
  - kontrola fizycznego nośnika (S006).
- **Kryteria sukcesu:** bramki z §7 przechodzą, a właściciel przechodzi na pendrive'ach scenariusz init → mount → zapis → unmount → backup → verify bez terminala.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `cmd/xordock/app.go` | kod: polecenia `vaults --json`, `volumes --json`, `points --json` dla aplikacji | do utworzenia |
| `internal/vault/peek.go` | kod: odczyt stanu sejfu bez blokady (dla zamontowanego) | do utworzenia |
| `internal/fusemount/mount.go` | kod: eksport `IsMounted` | do zmiany |
| `macos/Package.swift` | Swift Package: `XordockCore` (logika) + `XordockApp` (SwiftUI) | do utworzenia |
| `macos/Sources/XordockCore/*` | kod: model JSON, uruchamianie CLI, reguły przypomnień | do utworzenia |
| `macos/Sources/XordockApp/*` | kod: widoki SwiftUI | do utworzenia |
| `macos/Tests/XordockCoreTests/*` | testy (Swift Testing) | do utworzenia |
| `scripts/build-app.sh` | skrypt: składa `build/xordock.app` z CLI w `Contents/Resources` | do utworzenia |

## 5. Functional Domain Changes

- **Kontrakt CLI ↔ UI:**
  - JSON na stdout tylko dla poleceń zapytań (`vaults`, `volumes`, `points`). Akcje (init, mount, unmount, snapshot create, backup, recover …) zwracają tekst, a błąd to kod ≠ 0 i komunikat na stderr, który UI pokazuje bez zmian.
  - Każde pole JSON jest opcjonalne po stronie Swifta, a nieznane pola są ignorowane.
- **Bez powłoki:** UI uruchamia CLI przez `Process` z tablicą argumentów, nigdy przez `sh -c`, więc nazwy sejfów i ścieżki nie są interpretowane.
- **Montowanie:**
  - UI uruchamia `xordock mount --name N` jako proces potomny z wyjściem do `~/Library/Logs/xordock/N.log`, a nie do potoku. Proces żyje po zamknięciu UI, a zerwany potok nie zabije go sygnałem SIGPIPE.
  - Stan „zamontowany” UI ustala przez `vaults --json`, które porównuje urządzenie punktu montowania z katalogiem nadrzędnym, a nie przez śledzenie procesu.
- **Odświeżanie:** na powiadomienia NSWorkspace o montowaniu i odmontowaniu woluminów, po każdej akcji i co 30 s.
- **Stan zamontowanego sejfu:**
  - Mount trzyma flock na A, więc `vaults` czyta stan przez `vault.Peek`: superblok i indeks bez blokady, bez GC, bez recovery.
  - Obiekty są niezmienne, ale GC procesu montowania może usunąć stary indeks w trakcie odczytu. Wtedy `Peek` zwraca błąd, a UI pokazuje „szczegóły niedostępne” zamiast fałszywego stanu.
- **Backup:**
  - Migawka i backup wymagają odmontowanego sejfu (S004 A-11). Przycisk pyta o zgodę na odmontowanie.
  - Kartę zapasową wybiera się z listy woluminów. Gdy dla sejfu jest już karta zapasowa (rola `backup` z tym identyfikatorem), jest wybrana domyślnie. UI nie zapisuje na żaden nośnik bez wyboru użytkownika (S004 §7).
- **Odzyskiwanie:**
  - Działa bez karty roboczej.
  - Eksport wymaga wskazania katalogu nadrzędnego i nowej nazwy, bo CLI odmawia zapisu do istniejącego katalogu. Ostrzeżenie: eksport jest jawny.
- **Teksty:** maski opisane jako ochrona **praktyczna** (CG-14), ostrzeżenie o braku FileVault, ostrzeżenie „utrata karty bez backupu = utrata danych”.
- **Dane:** format sejfu bez zmian. UI nie czyta i nie zapisuje udziałów ani jawnej treści, robi to tylko CLI.

## 6. Critical Path

1. Polecenia JSON w CLI z testami Go.
2. `XordockCore`: dekodowanie, uruchamianie procesów, reguła przypomnienia, z testami.
3. Widoki SwiftUI: pasek menu, okno z listą sejfów, kreator, backup, odzyskiwanie.
4. `scripts/build-app.sh`, test ręczny na pendrive'ach.

Gotowe, gdy bramki G1–G9 są zaliczone.

## 7. Control Gates

| # | Bramka | Warunek zaliczenia | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | Testy Go | `go test -race ./...` zielone, w tym nowe testy `vaults`/`volumes`/`Peek` | test | commit |
| G2 | Testy Swift | `swift test` zielone: dekodowanie JSON, przypomnienie 7 dni, argumenty poleceń | test | commit |
| G3 | Budowanie | `scripts/build-app.sh` tworzy podpisane ad hoc `build/xordock.app` z CLI w środku | polecenie | commit |
| G4 | **Negatywna:** bez powłoki | w kodzie Swift brak `/bin/sh`, `bash`, `-c` | grep w teście | commit |
| G5 | **Negatywna:** UI nie dotyka udziałów | kod Swift nie otwiera plików w katalogach sejfów (tylko `Process` + log) | przegląd + grep | commit |
| G6 | Montowanie przeżywa zamknięcie UI | po zamknięciu aplikacji wolumin dalej działa | E2E ręczny | wydanie |
| G7 | Stan zamontowanego sejfu | `vaults --json` przy zamontowanym sejfie zwraca `mounted:true` i ostatni backup | test Go + E2E | commit |
| G8 | CG-14 | teksty UI mówią o ochronie praktycznej | przegląd | commit |
| G9 | Log | log montowania bez nazw plików (CG-12) | E2E | wydanie |

## 8. Expected Outcome

- Codzienna praca bez terminala.
- Skutek uboczny: dochodzi drugi język (Swift) i drugi proces. Silnik bez zmian.
- Wycofanie: usunięcie `macos/` i poleceń JSON nie zmienia formatu ani CLI.

## 9. Additional Information

- **Odrzucone:**
  - Go jako c-archive w procesie UI: awaria UI zrywa montowanie, a dwa runtime'y działają w jednym procesie.
  - Projekt `.xcodeproj`: Swift Package + skrypt wystarczą i nie wymagają konta Apple.
- **Zależność:** macFUSE 5.5 (CLI linkuje `/usr/local/lib/libfuse3`). Aplikacja nie dołącza macFUSE (S004 A-12).
- **Dług:**
  - brak podpisu Developer ID: Gatekeeper może ostrzec przy pierwszym uruchomieniu z innego Maca;
  - brak testów UI (XCUITest wymaga projektu Xcode).

## Realizacja (2026-11-22)

- **Budowanie:**
  - Swift Package (`macos/Package.swift`, Swift 6.3, narzędzia 5.10, macOS 14+) zamiast projektu Xcode.
  - `swift test` wymaga `DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer`, bo Command Line Tools nie mają modułu Testing. Skrypt `scripts/build-app.sh` ustawia to sam.
- **CLI:**
  - `vaults` (stan wszystkich sejfów + FileVault + opis masek), `volumes` (woluminy w `/Volumes` z FS, wolnym miejscem, flagą `systemDisk` i sejfami na nośniku), `points --spare`. Czasy w RFC 3339 z dokładnością do sekundy.
  - `systemDisk` to wstępna ochrona w UI (ten sam dysk całościowy co `/`). Pełna kontrola fizycznego nośnika należy do S006.
- **Silnik:** `vault.Peek` czyta stan bez blokady. Gdy superbloki wskazują różne commity, zwraca `ErrBusy`. Najnowszy punkt READY liczy wspólna funkcja `lastReady`, z której korzysta też `LastBackup`.
- **Nazwa sejfu (R7-01):** walidowana w CLI (`--name`, `--new-name`) i w UI. Nie może być pusta, mieć więcej niż 64 znaki, zaczynać się kropką, zawierać `/`, `:` ani znaków sterujących, zaczynać się ani kończyć spacją. Wcześniej `--name ../x` wyprowadzało A, B i punkt montowania poza ich katalogi.
- **Lokalizacja CLI (R7-03):** w pakiecie `.app` wyłącznie `Contents/Resources/xordock`. `XORDOCK_CLI` i `bin/xordock` działają tylko poza pakietem (`swift run`, testy).
- **Montowanie:** proces `xordock mount` startuje z wyjściem do `~/Library/Logs/xordock/<nazwa>.log` (katalog 0700, plik 0600). Aplikacja czeka do 15 s, aż wolumin się pojawi, a potem otwiera go w Finderze.
- **Ikona w pasku menu:** `lock.shield` gdy wszystko w porządku, `lock.open` gdy coś jest zamontowane, `exclamationmark.shield` przy błędzie sejfu, wyłączonym FileVault albo wymaganym backupie.
- **Bramki:**
  - G1: `go test -race ./...` (nowe: `TestPeekWhileOpen`, `TestCollectVaults`, `TestWholeDisk`, `TestValidName`).
  - G2, G4, G5: 9 testów Swift w `macos/Tests/XordockCoreTests/CoreTests.swift`. G4 i G5 sprawdza test przeszukujący źródła.
  - G3: skrypt i `codesign --verify --strict`.
  - G7: `TestPeekWhileOpen` (odczyt przy trzymanej blokadzie).
  - G8: teksty w kreatorze, w szczegółach sejfu i w polu `masks`.
