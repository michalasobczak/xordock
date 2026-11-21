# Jak bezpiecznie używać xordocka

> **Prototyp.** Maski pochodzą z CSPRNG: poufność jest obliczeniowa, **bez** gwarancji odporności na P = NP.

## Pierwsze uruchomienie

```bash
go build -o bin/xordock ./cmd/xordock
./bin/xordock init --card /Volumes/xordock --name sejf
./bin/xordock mount --name sejf
```

Katalog pojawia się w `~/xordock/sejf` i w Finderze jako wolumen sieciowy `sejf`. Proces `mount` działa na pierwszym planie. Odmontowanie: Ctrl+C, „Wysuń” w Finderze albo `umount ~/xordock/sejf`.

- Udział A: `~/Library/Application Support/xordock/sejf`
- Udział B: `/Volumes/<karta>/xordock/sejf`. Kartę rozpoznajemy po identyfikatorze sejfu, nie po nazwie wolumenu.

## Karta SD

- **Wyjęcie karty** w trakcie pracy odmontowuje katalog w ciągu sekundy. Pliki zamknięte przed wyjęciem są bezpieczne. Zmiany w plikach otwartych w tym momencie przepadają: aplikacja dostanie błąd.
- **Utrata karty = utrata danych.** Kopia zapasowa: przy odmontowanym sejfie skopiuj katalog `/Volumes/<karta>/xordock/sejf` na drugą kartę i przechowuj ją w innym miejscu niż Maca. Kopia samego udziału A (np. Time Machine) **nie wystarczy** do odzyskania danych.
- Karta wycofana z użytku: nadpisz ją w całości albo zniszcz. Zwykłe usunięcie plików nie usuwa masek z nośnika (F-04).
- Zwykłą kartę SD da się skopiować. Nie jest to token sprzętowy.

## Gdzie jawne dane mogą trafić poza sejf (FR-25, FR-26)

xordock nie kontroluje innych programów. Nie obiecujemy, że usuwają swoje kopie.

- **Cache agenta WebDAV macOS** (`/private/tmp/.webdavcache.*`): treść otwieranych plików trafia na SSD na czas montowania (F-01). Dlatego `mount` wymaga FileVault. Usunie to S002.
- **Autozapis i pliki tymczasowe** aplikacji biurowych (często w `~/Library/Containers/…` albo `/private/var/folders/…`).
- **Miniatury QuickLook i podglądy Findera** (`~/Library/Caches`, cache QuickLook).
- **Ostatnie dokumenty**: nazwy plików w historii aplikacji i systemu.
- **RAM i swap.** Swap na Apple Silicon jest szyfrowany.

Zalecenia: włączony FileVault (FR-27), otwieranie plików z sejfu w możliwie prostych programach, po pracy odmontowanie katalogu.

## Diagnostyka i wyjście

```bash
./bin/xordock fsck --name sejf
./bin/xordock export --name sejf --to ~/Desktop/eksport-sejfu
```

`fsck` sprawdza sumy obu udziałów każdego obiektu bez składania treści. `export` (rollback z S001 §8) zapisuje jawne pliki do nowego katalogu. Te pliki **nie są już chronione**.
