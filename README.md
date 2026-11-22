# xordock

> Aplikacja na macOS, która udostępnia zwykły katalog w Finderze, a jego zawartość przechowuje jako dwa udziały XOR: jeden na wewnętrznym SSD Maca, drugi na wyjmowanej karcie SD. Bez karty danych nie da się odtworzyć.

## Stan obecny (2026-11-22)

**Prototyp S004: katalog w Finderze, migawki, backup na kartę zapasową, odzyskiwanie po utracie karty.** Nie do danych produkcyjnych.

- Wolumen `/Volumes/xordock-<nazwa>` w Finderze przez macFUSE 5.5 z backendem FSKit: bez kext, SIP/AMFI włączone, bez płatnego konta Apple. Działa: tworzenie, odczyt, zapis, obcinanie, rename, usuwanie, katalogi.
- Każdy blok (1 MiB) i indeks z nazwami plików są dzielone XOR: A na SSD, B na karcie roboczej. Każdy zapis tworzy nowy obiekt z nową maską. Commit przez superbloki A→B z odzyskiwaniem po awarii.
- Maski: **CSPRNG macOS (`SecRandomCopyBytes`)**. Ochrona praktyczna, **nie** informacyjno-teoretyczna i **bez** odporności na P = NP.
- Wyjęcie karty roboczej: wolumen zablokowany (EIO) i odmontowany w ≤ 1,25 s.
- Migawki tylko udziałów xordocka (bez APFS), backup brakujących udziałów B na kartę zapasową z punktem READY, odzyskiwanie z migawki A + karty zapasowej do nowego sejfu z nowymi maskami albo do jawnego eksportu.
- **Kopia udziału A w iCloud Drive** (S008) przy każdym backupie. Można stracić pojedynczo SSD, kartę roboczą albo zapasową.
- **Aplikacja** `xordock.app` (S007): ikona w pasku menu i okno z zakładaniem sejfu, montowaniem, backupem i odzyskiwaniem, bez terminala.
- Znana luka (S006, szkic): kontrola „A i B na różnych nośnikach” porównuje woluminy, nie fizyczne dyski. Obraz dysku albo druga partycja na tym samym dysku przechodzą.
- Brak: testu nocnego 1 GiB i fuzzingu (CP-09), uprawnień plików, `xattr`, symlinków, hasła, wykrywania celowej podmiany udziału. Otwarte znalezisko R4-03: błąd zatwierdzenia przy `close` nie dociera do aplikacji (macFUSE/FSKit).
- Wydajność (SSD + karta SD HFS+, przez Findera): zapis ~10 MiB/s, mały plik ~220 ms.
- Środowisko testowe: macOS 27.0, Apple Silicon, Go 1.27.4, Xcode 27.2, macFUSE 5.5.0 (libfuse3 3.18.3).

## Użycie

### Aplikacja

```bash
./scripts/build-app.sh
open build/xordock.app
```

Ikona kłódki pojawi się w pasku menu, a „Otwórz xordock…” otwiera okno. Pliki A należą do Ciebie, więc nic nie uruchamiaj przez `sudo`.

### CLI

Wymagane: macFUSE 5.5+ (`brew install --cask macfuse`, instaluje użytkownik; xordock go nie dołącza, zob. licencja macFUSE) i włączone rozszerzenie w Ustawieniach systemowych → Ogólne → Rozszerzenia i elementy logowania → Rozszerzenia systemu plików.

```bash
go build -o bin/xordock ./cmd/xordock
./bin/xordock init --card /Volumes/xordock --name sejf
./bin/xordock mount --name sejf            # do wysunięcia w Finderze / Ctrl-C
./bin/xordock status --name sejf
./bin/xordock snapshot create --name sejf
./bin/xordock backup --name sejf --spare /Volumes/ZAPAS   # + kopia A do iCloud Drive
./bin/xordock recover --name sejf --spare /Volumes/ZAPAS restore --card /Volumes/NOWA
```

Pełna lista: `xordock help`. Zasady: [docs/uzytkowanie.md](docs/uzytkowanie.md), backup i odzyskiwanie: [docs/recovery.md](docs/recovery.md), format: [docs/format-v1.md](docs/format-v1.md), model zagrożeń: [docs/threat-model.md](docs/threat-model.md).

## Architektura

| Pakiet | Rola |
| --- | --- |
| `internal/split` | `A = M ⊕ R`, `B = R`; maski z `SecRandomCopyBytes` (cgo) |
| `internal/vault` | format udziałów v1, bloki 1 MiB, indeks dzielony XOR, commit A→B, odzyskiwanie po awarii, GC z przypięciami migawek, migawki, backup na kartę zapasową, odzyskiwanie, `fsck`, `flock` |
| `internal/fusefs` | operacje systemu plików na silniku: bufory zapisu w RAM, commit przy zamknięciu/fsync, stan niedostępny po wyjęciu karty; bez kryptografii |
| `internal/fusemount` | most cgo do libfuse3 z macFUSE (`backend=fskit`), nadzór karty, odmontowanie |
| `cmd/xordock` | CLI; `vaults`/`volumes`/`points` dają JSON dla aplikacji |
| `macos/` | aplikacja SwiftUI (`XordockCore`: model, uruchamianie CLI, reguły; `XordockApp`: widoki), bez dostępu do udziałów |

Zależności: biblioteka standardowa Go, cgo do Security.framework i libfuse3 (macFUSE).

## Testy

```bash
go test -race ./...
(cd macos && DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer swift test)
```

Testy bramek CG-01–CG-12 S004 są w `internal/*/…_test.go` (mapowanie: S004 §9 „Bramki i dowody”). E2E na karcie SD i obrazach dysku opisuje CHANGELOG. Pomiary silnika: `XORDOCK_PERF_A=<katalog na SSD> XORDOCK_PERF_B=<katalog na karcie> go test -run TestPerf -v ./internal/vault`.

## Jak pracujemy

Metodyka [Rondo + Context-Driven-Development](https://workwithrondo.com):

| Artefakt | Rola |
| --- | --- |
| `specs/` | zamiar: cel, zakres, reguły i bramki każdej zmiany |
| `CHANGELOG.md` | co faktycznie dostarczono |
| testy (`*_test.go`) | dowód tego, co opisuje CHANGELOG |
| `README.md` | stan „tu i teraz” |
| `MANIFEST.md` | zasady inżynierskie projektu |
| `docs/bezpieczenstwo/` | analizy bezpieczeństwa po każdej rundzie (najnowsza: runda S008) |

## Specyfikacje

| Nr | Tytuł | Status |
| --- | --- | --- |
| [S001](specs/S001-xordock-secure-folders.md) | xordock Secure Folders for macOS | zastąpiona przez S004 |
| [S002](specs/S002-integracja-bez-cache.md) | Katalog w Finderze przez własny FSKit | porzucona |
| [S003](specs/S003-ochrona-katalogu-z-findera.md) | Ochrona katalogu z poziomu Findera | szkic |
| [S004](specs/S004-xordock-macos-mvp.md) | XorDock: dwa udziały, migawki, backup i odzyskiwanie | dostarczona CP-01–CP-08 (CP-08 przez S007); część CP-09 otwarta |
| [S005](specs/S005-poprawki-bezpieczenstwa-S004.md) | Poprawki z analizy bezpieczeństwa rundy S004 | dostarczona |
| [S006](specs/S006-fizyczny-nosnik.md) | Udziały A i B na różnych fizycznych nośnikach | szkic (decyzje zebrane) |
| [S007](specs/S007-aplikacja-macos.md) | Aplikacja macOS: pasek menu i okno | dostarczona; G6, G9 do testu właściciela |
| [S008](specs/S008-kopia-A-w-icloud.md) | Kopia udziału A w iCloud Drive | dostarczona; G7 do testu właściciela |
