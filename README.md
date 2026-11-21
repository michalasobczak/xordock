# xordock

> Aplikacja na macOS, która udostępnia zwykły katalog w Finderze, a jego zawartość przechowuje jako dwa udziały XOR: jeden na wewnętrznym SSD Maca, drugi na wyjmowanej karcie SD. Bez karty danych nie da się odtworzyć.

## Stan obecny (2026-11-22)

**Działający prototyp S001.** Z poziomu CLI można utworzyć sejf, zamontować go jako katalog widoczny w Finderze, pracować na plikach, odmontować, sprawdzić spójność i wyeksportować dane. Wyjęcie karty odmontowuje katalog w ≤ 1 s.

- Maski: **CSPRNG**. Poufność jest obliczeniowa, **bez** gwarancji odporności na P = NP (S001 IT-01–IT-04).
- Integracja z macOS: lokalny WebDAV + `mount_webdav`. **Nie do produkcji:** agent WebDAV macOS buforuje jawną treść na SSD (F-01). Dlatego `mount` wymaga FileVault. Rozwiązanie: S002.
- Brak: hasła (FR-05), wykrywania celowej podmiany udziału (R-04), automatycznych kopii karty, obsługi z Findera bez terminala (S003).
- Wydajność (SSD + karta SD): zapis ~10 MiB/s, odczyt ~600 MiB/s, mały plik ~120–215 ms, montowanie 1 s.

## Użycie

```bash
go build -o bin/xordock ./cmd/xordock
./bin/xordock init --card /Volumes/xordock --name sejf
./bin/xordock mount --name sejf        # katalog: ~/xordock/sejf, Ctrl+C odmontowuje
./bin/xordock fsck --name sejf
./bin/xordock export --name sejf --to ~/Desktop/eksport
```

Zasady bezpiecznego używania, kopie karty i ograniczenia: [docs/uzytkowanie.md](docs/uzytkowanie.md). Model zagrożeń: [docs/threat-model.md](docs/threat-model.md).

## Architektura

| Pakiet | Rola |
| --- | --- |
| `internal/split` | `A = M ⊕ R`, `B = R`; źródło masek (CSPRNG) |
| `internal/vault` | format udziałów, bloki 1 MiB, indeks dzielony XOR (nazwy plików), commit przez superbloki A→B, odzyskiwanie po awarii, GC, `fsck`, blokada `flock` |
| `internal/davfs` | `webdav.FileSystem` nad sejfem; jawne bufory tylko w RAM, zatwierdzanie przy zamknięciu pliku |
| `internal/mount` | serwer na 127.0.0.1 z losowym tokenem, `mount_webdav`, nadzór nad kartą |
| `cmd/xordock` | CLI: `init`, `mount`, `fsck`, `export` |

Zależności: Go 1.27, `golang.org/x/net/webdav`.

## Testy

```bash
go test -race ./...
```

Pomiary na prawdziwych nośnikach: `XORDOCK_PERF_A=<katalog na SSD> XORDOCK_PERF_B=<katalog na karcie> go test -run TestPerf -v ./internal/vault`. Mapowanie bramek CG-01–CG-11 na testy: S001 §9.9.

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
| [S002](specs/S002-integracja-bez-cache.md) | Integracja z macOS bez jawnego cache na dysku | szkic |
| [S003](specs/S003-ochrona-katalogu-z-findera.md) | Ochrona katalogu z poziomu Findera | szkic |
