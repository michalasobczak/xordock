# xordock

> Aplikacja na macOS, która udostępnia zwykły katalog w Finderze, a jego zawartość przechowuje jako dwa udziały XOR: jeden na wewnętrznym SSD Maca, drugi na wyjmowanej karcie SD. Bez karty danych nie da się odtworzyć.

## Stan obecny (2026-11-22)

**Silnik sejfu gotowy, montowanie w Finderze w budowie (S002, FSKit).**

- Działa z poziomu CLI: tworzenie sejfu (`init`), kontrola spójności bez składania treści (`fsck`) i eksport (`export`).
- Silnik (`internal/vault`) jest przetestowany, także na prawdziwej karcie SD (S001 §9.9): podział XOR, nowa maska przy każdym zapisie, nazwy plików w indeksie dzielonym XOR, odzyskiwanie po awarii, stałe numery plików pod FSKit.
- Prototyp z montowaniem przez WebDAV (commit `98e5fdc`) usunięto, bo macOS zapisywał jawną treść w cache na SSD (F-01). NFS odrzucono. Docelowo: natywne rozszerzenie FSKit.
- Maski: **CSPRNG**. Poufność jest obliczeniowa, **bez** gwarancji odporności na P = NP (S001 IT-01–IT-04).
- Brak: montowania w Finderze (S002), hasła (FR-05), wykrywania celowej podmiany udziału (R-04), obsługi z Findera (S003).
- Wydajność silnika (SSD + karta SD): zapis ~10 MiB/s, odczyt ~600 MiB/s, mały plik ~120 ms.

## Użycie

```bash
go build -o bin/xordock ./cmd/xordock
./bin/xordock init --card /Volumes/xordock --name sejf
./bin/xordock fsck --name sejf
./bin/xordock export --name sejf --to ~/Desktop/eksport
```

Zasady bezpiecznego używania i kopie karty: [docs/uzytkowanie.md](docs/uzytkowanie.md). Model zagrożeń: [docs/threat-model.md](docs/threat-model.md).

## Architektura

| Pakiet | Rola |
| --- | --- |
| `internal/split` | `A = M ⊕ R`, `B = R`; źródło masek (CSPRNG) |
| `internal/vault` | format udziałów, bloki 1 MiB, indeks dzielony XOR (nazwy plików, stałe numery plików), commit przez superbloki A→B, odzyskiwanie po awarii, GC, `fsck`, blokada `flock` |
| `cmd/xordock` | CLI: `init`, `fsck`, `export` |

Zależności: tylko biblioteka standardowa Go 1.27.

## Testy

```bash
go test -race ./...
```

Pomiary na prawdziwych nośnikach: `XORDOCK_PERF_A=<katalog na SSD> XORDOCK_PERF_B=<katalog na karcie> go test -run TestPerf -v ./internal/vault`.

## Jak pracujemy

Metodyka [Rondo + Context-Driven-Development](https://workwithrondo.com):

| Artefakt | Rola |
| --- | --- |
| `specs/` | zamiar: cel, zakres, reguły i bramki każdej zmiany |
| `CHANGELOG.md` | co faktycznie dostarczono |
| testy (`*_test.go`) | dowód tego, co opisuje CHANGELOG |
| `README.md` | stan „tu i teraz” |
| `MANIFEST.md` | zasady inżynierskie projektu |
| `docs/bezpieczenstwo/` | analizy bezpieczeństwa po każdej rundzie |

## Specyfikacje

| Nr | Tytuł | Status |
| --- | --- | --- |
| [S001](specs/S001-xordock-secure-folders.md) | xordock Secure Folders for macOS | dostarczona (prototyp) |
| [S002](specs/S002-integracja-bez-cache.md) | Katalog w Finderze przez FSKit | szkic |
| [S003](specs/S003-ochrona-katalogu-z-findera.md) | Ochrona katalogu z poziomu Findera | szkic |
