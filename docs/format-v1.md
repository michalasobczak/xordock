# Format v1

Kod: `internal/vault/format.go`, `vault.go`, `snapshot.go`, `backup.go`. Wersja formatu: `1` (pole `format` w nagłówku). Pola dodane w S004 (`role`, `media`, migawki w indeksie) są opcjonalne, więc sejfy z S001 otwierają się bez migracji.

## Korzenie

| Korzeń | Gdzie | Zawartość |
|---|---|---|
| A | SSD, domyślnie `~/Library/Application Support/xordock/<nazwa>` | `xordock.json`, `super`, `obj/`, `lock` |
| B (karta robocza) | `<karta>/xordock/<nazwa>` | `xordock.json`, `super`, `obj/` |
| karta zapasowa | `<karta zapasowa>/xordock/<nazwa>` | `xordock.json` (rola `backup`), `obj/` (kopie udziałów B), `points/` |

## Nagłówek `xordock.json` (jawny)

`format`, `vault` (identyfikator pary, 128 bitów, niezwiązany z nazwą), `side` (`A`/`B`), `name`, `block_size` (1 MiB), `created`, `masks` (opis źródła masek), opcjonalnie `role` (`backup`) i `media` (identyfikator karty zapasowej).

## Udział obiektu `obj/xx/<oid>`

```
"XDK1" | strona (1 B) | vault (16 B) | oid (16 B) | długość (uint32 BE) | dane | SHA-256(wszystko wcześniej)
```

- Obiekt to blok pliku (do 1 MiB) albo indeks. Strona A: `M ⊕ R`, strona B: `R`. Maska `R` jest nowa dla każdego obiektu.
- Obiekty są niezmienne: każdy zapis tworzy nowy `oid` (losowy, nie skrót treści).
- Suma kontrolna obejmuje tylko własny udział, nigdy drugi (D-05, I-03).

## Indeks (obiekt dzielony XOR)

JSON: `e` (ścieżka → wpis: `i` numer pliku, `d` katalog, `s` rozmiar, `t` czas, `b` lista oid bloków), `n` (następny numer pliku), `s` (migawki: `id`, `q` generacja, `x` oid indeksu, `t` czas, `p` przypięcia punktów: `m` nośnik, `p` punkt, `r` READY, `t` czas).

Ścieżki są kanoniczne (`/a/b`), każdy wpis ma katalog nadrzędny; indeks, który tego nie spełnia, jest odrzucany (S005).

## Superblok `super` (jawny)

Linia JSON `{format, vault, side, cur:{seq, index}, prev:{seq, index}}` i linia z jej SHA-256.

## Commit (T-01, A-05)

1. Nowe obiekty A i B (tmp + fsync + rename).
2. fsync katalogów, bariera `F_FULLFSYNC` na obu nośnikach.
3. Superblok A (stan przygotowany), potem superblok B (zatwierdzony).

Przy otwarciu: A = B → stan; A o jeden dalej niż B → dokończenie (gdy obiekty kompletne) albo wycofanie; inne stany → jawny błąd. GC usuwa obiekty nieosiągalne z `cur`, `prev` i wszystkich migawek (najpierw B, potem A); przy nieczytelnej migawce GC jest wstrzymane.

## Punkt odzyskiwania `points/<id>` (karta zapasowa, jawny)

Linia JSON `{format, vault, media, id, snapshot, seq, index, created, objects, bytes, state:"READY"}` i linia SHA-256. Istnienie rekordu = punkt READY; zapisywany po utrwaleniu i weryfikacji wszystkich obiektów. Nie zawiera nazw plików.
