# Jak bezpiecznie używać xordocka

> **Prototyp.** Nie przechowuj prawdziwych tajnych dokumentów przed przejściem bramek awarii i odzyskiwania na własnym sprzęcie. Maski pochodzą z CSPRNG macOS: ochrona praktyczna, **bez** gwarancji informacyjno-teoretycznej i odporności na P = NP.

## Pierwsze uruchomienie

Wymagania: macFUSE 5.5+ (`brew install --cask macfuse`) i włączone rozszerzenie: Ustawienia systemowe → Ogólne → Rozszerzenia i elementy logowania → Rozszerzenia systemu plików → macFUSE. Bez kext, bez obniżania zabezpieczeń.

```bash
go build -o bin/xordock ./cmd/xordock
./bin/xordock init --card /Volumes/xordock --name sejf
./bin/xordock mount --name sejf          # wolumen /Volumes/xordock-sejf w Finderze
```

Montowanie działa do wysunięcia wolumenu w Finderze, `xordock unmount --name sejf` albo Ctrl-C. Wyjęcie karty w trakcie pracy blokuje wolumen (błędy EIO) i odmontowuje go w ok. 1 s; niezatwierdzone zmiany przepadają, ostatnia zatwierdzona wersja zostaje. Zmiany pliku są zatwierdzane przy jego zamknięciu albo `fsync`.

Ograniczenia: brak uprawnień (pliki 0600), `xattr`, symlinków i twardych linków; zamiana plików miejscami (`RENAME_SWAP`) zwraca błąd.

- Udział A: `~/Library/Application Support/xordock/sejf`
- Udział B: `/Volumes/<karta>/xordock/sejf`. Kartę rozpoznajemy po identyfikatorze sejfu, nie po nazwie wolumenu.

## Karta SD

- **Utrata karty bez backupu = utrata danych.** Rób regularnie migawkę i backup na kartę zapasową: [recovery.md](recovery.md). Kopia samego udziału A (np. Time Machine) **nie wystarczy** do odzyskania danych.
- Karta wycofana z użytku: nadpisz ją w całości albo zniszcz. Zwykłe usunięcie plików nie usuwa masek z nośnika (F-04).
- Zwykłą kartę SD da się skopiować. Nie jest to token sprzętowy.

## Gdzie jawne dane mogą trafić poza sejf (FR-25, FR-26)

xordock nie kontroluje innych programów. Nie obiecujemy, że usuwają swoje kopie.

- **Autozapis i pliki tymczasowe** aplikacji biurowych (często w `~/Library/Containers/…` albo `/private/var/folders/…`).
- **Miniatury QuickLook i podglądy Findera** (`~/Library/Caches`, cache QuickLook).
- **Ostatnie dokumenty**: nazwy plików w historii aplikacji i systemu.
- **RAM i swap.** Swap na Apple Silicon jest szyfrowany.
- **Spotlight, crash dumpy, Time Machine, iCloud**, jeśli aplikacje zapiszą kopie poza wolumenem.

Zalecenia: włączony FileVault (FR-27), otwieranie plików z sejfu w możliwie prostych programach, po pracy odmontowanie katalogu.

## Diagnostyka i wyjście

```bash
./bin/xordock fsck --name sejf
./bin/xordock export --name sejf --to ~/Desktop/eksport-sejfu
```

`fsck` sprawdza sumy obu udziałów każdego obiektu bez składania treści. `export` (rollback z S001 §8) zapisuje jawne pliki do nowego katalogu. Te pliki **nie są już chronione**.
