# CHANGELOG

Chronologiczny zapis tego, co faktycznie dostarczono. Wpisy są uporządkowane według dat, najnowsze na górze.

## 2026-11-22

### S005: poprawki z analizy bezpieczeństwa rundy S004

- `backup prune` sprawdza oba udziały każdego obiektu najnowszego punktu, zanim usunie cokolwiek starszego (R4-01). Test `TestPruneRefusesIncompleteNewest`.
- Indeks wczytany z udziałów jest walidowany (ścieżki kanoniczne, katalog nadrzędny), a eksport odmawia ścieżek spoza katalogu docelowego (R4-02). Test `TestIndexPathsValidated`.
- Analiza: `docs/bezpieczenstwo/2026-11-22-runda-S004.md`. F-01 i F-02 z rundy S001 zamknięte. Otwarte R4-03 (średnie, złagodzone): błąd zatwierdzenia przy `close` nie dociera do aplikacji przez FSKit.

### S004: katalog w Finderze przez macFUSE/FSKit, migawki, backup i odzyskiwanie

- Specyfikacja właściciela wpisana jako S004. Rdzeń zostaje w Go (decyzja właściciela, odstępstwo od ADR-001), decyzje A-01–A-12 w S004 §9. S001 zastąpiona, S002 porzucona; niezacommitowany projekt `macos/` z własnym modułem FSKit usunięty.
- `internal/split`: maski z `SecRandomCopyBytes` (cgo, kontrola kodu wyniku), opis „ochrona praktyczna, bez gwarancji informacyjno-teoretycznej”; deterministyczne źródło TEST_ONLY tylko w testach.
- `internal/vault`:
  - Migawki w indeksie dzielonym XOR (`CreateSnapshot`, `Snapshots`, `DeleteSnapshot`, `SnapshotView`, `Changes`). GC respektuje migawki i wstrzymuje się, gdy indeks migawki jest nieczytelny.
  - Backup na kartę zapasową (rola `backup` w nagłówku): przypięcie, kopia brakujących udziałów B, weryfikacja, punkt READY. Sprawdzanie miejsca przed zmianą, `PruneBackup`.
  - Odzyskiwanie (`OpenRecovery`, `Verify`, `View`), `ExportTo`, `CopyInto` (nowy sejf, nowe maski). `FindCards`, `fsck` obejmuje migawki. Karta zapasowa odrzucana jako robocza.
- `internal/fusefs`: system plików nad silnikiem: bufory zapisu w RAM, spill pełnych bloków powyżej 64 MiB, commit przy flush/fsync/release, obcinanie, rename (EXCL; SWAP → ENOTSUP), stan niedostępny po wyjęciu karty z czyszczeniem buforów.
- `internal/fusemount`: most cgo do libfuse3 z macFUSE (`backend=fskit`), nadzór karty co 1 s z wymuszonym odmontowaniem, wykrycie nieudanego montowania. `chmod`/`chown` bez skutku (A-06).
- CLI: `mount`, `unmount`, `status`, `snapshot create|list|rm`, `backup [prune]`, `recover list|verify|export|restore`; karty rozpoznawane po identyfikatorze sejfu i roli.
- Testy: 20 nowych, razem 44 plus `TestPerf` (migawki, GC, backup z awariami na 3 etapach, brak miejsca, retencja, odzyskiwanie, CG-09, brak jawnej treści na karcie zapasowej, adapter: model losowych zapisów, spill, katalogi, obcinanie, wyjęcie karty, usunięcie otwartego pliku; źródło masek TEST_ONLY i błąd generatora). Dwa z nich (S005) dochodzą w osobnym wpisie wyżej.
- E2E (macOS 27.0, macFUSE 5.5.0, karta SD HFS+, obrazy dysku jako karta zapasowa i nowa karta):
  - montowanie w Finderze, mkdir, zapis i `cmp` 5 MB i 30 MiB, dopisywanie, rename między katalogami, 30 małych plików, usuwanie, `truncate`;
  - brak markera i nazw w A, B, na karcie zapasowej, w nowym sejfie, `/private/tmp` i `/private/var/folders`;
  - symulowane wyjęcie karty: odmontowanie po ≤ 1,25 s;
  - `fsck`, migawka, backup, zmiana po backupie, `recover verify/export/restore` bez karty roboczej: dokładne bajty punktu, bez późniejszej zmiany;
  - sejfy testowe usunięte z karty.
- Poprawione w trakcie E2E: `chmod` zwracający ENOTSUP blokował `mkdir`/`create` przez FSKit; CLI zgłaszało montowanie i odmontowanie mimo nieudanego montowania albo awaryjnego odmontowania.
- Dokumentacja: `docs/format-v1.md`, `docs/recovery.md`, aktualizacja `docs/threat-model.md`, `docs/uzytkowanie.md`, `MANIFEST.md` (stack).

### S002 krok 1: próba FSKit

- `macos/`: projekt Xcode (aplikacja-kontener `pl.indatify.xordock` + rozszerzenie FSKit `pl.indatify.xordock.fs`), moduł „hello” tylko do odczytu. Kompiluje się (Xcode 27.2, macOS SDK 27.2).
- Podpis: certyfikat Apple Development i pośredni WWDR G3 zainstalowane przez właściciela.
- **Blokada:** Personal Team nie obsługuje uprawnienia FSKit Module, więc Xcode nie tworzy profilu. Do dalszej pracy potrzebny płatny Apple Developer Program.

### S002 (przygotowanie): usunięcie WebDAV, kierunek FSKit

- Właściciel odrzucił WebDAV i NFS; integracja z Finderem ma być natywna (FSKit). Usunięto `xordock mount`, `internal/davfs`, `internal/mount` i zależność `golang.org/x/net`. Projekt nie ma zależności zewnętrznych.
- F-01 i F-02 z analizy rundy S001 przestały dotyczyć kodu (usunięta integracja). Wrócą do sprawdzenia przy S002.
- Próbny serwer NFSv3 napisany i porzucony przed pierwszym udanym montowaniem (nie trafił do repozytorium).
- `internal/vault`: stałe numery plików w indeksie (`Entry.Ino`, katalog główny = 1, uzupełniane w starych indeksach), operacje po numerze i z semantyką systemu plików: `ByIno`, `SetFileIno`, `SetModTime`, `Create`, `Remove` (unlink/rmdir), `RenameReplacing`. Testy: `TestInodesStable`, `TestReplaceAndRemoveSemantics`, `TestEnsureInosForOldIndex`.
- S002 przepisana na FSKit. Stanowisko: Xcode 27.2 z FSKit SDK; brak tożsamości do podpisywania.

### S001: prototyp xordock Secure Folders

- Specyfikacja S001 (szkic właściciela, nazwa robocza OpenTrust → xordock). Decyzje Q-01–Q-06 i FR-05 według rekomendacji, na polecenie właściciela (S001 §9.8).
- Go, moduł `xordock`, jedna zależność `golang.org/x/net/webdav`.
- `internal/split`: podział XOR, źródło masek CSPRNG z jawnym opisem braku gwarancji P = NP (CG-10).
- `internal/vault`: format udziałów v1 (magic, strona, sejf, obiekt, długość, SHA-256 własnego udziału), bloki 1 MiB, każdy zapis to nowy obiekt z nową maską, indeks z nazwami plików dzielony XOR. Commit: obiekty (fsync) → bariera F_FULLFSYNC → superblok A → superblok B. Odzyskiwanie: dokończenie albo wycofanie przerwanego commitu, inne stany to jawny błąd. GC starych wersji (najpierw B). `fsck` bez składania treści. Blokada `flock`.
- `internal/davfs`: system plików WebDAV nad sejfem: zapisy w dowolnych miejscach, dziury, spill pełnych bloków przy zapisie sekwencyjnym, zatwierdzanie przy zamknięciu.
- `internal/mount`: serwer na 127.0.0.1 z losowym tokenem w ścieżce, odrzucanie obcego `Host` i `Origin`, `mount_webdav`, nadzór nad kartą co 1 s (stan błędu + wymuszone odmontowanie).
- CLI `xordock init|mount|fsck|export`. `mount` wymaga FileVault.
- Testy: 27 testów jednostkowych i protokołu (plus `TestPerf` uruchamiany ręcznie), w tym 13 scenariuszy awarii na każdym etapie zapisu. E2E na prawdziwej karcie SD (HFS+) i SSD: zapis/odczyt 20 MB i 3 MB, rename, delete, 50 małych plików, ponowne montowanie, symulowane wyjęcie karty, blokada przy równoczesnym `fsck`, eksport, brak markera treści i nazwy pliku w obu udziałach.
- Wydajność: pierwsza wersja (blok 64 KiB) 1,4 MiB/s zapisu, po zmianie na 1 MiB i utrwalaniu katalogów raz na commit 10,4 MiB/s. Odczyt ~600 MiB/s.
- Analiza bezpieczeństwa rundy: `docs/bezpieczenstwo/2026-11-22-runda-S001.md`. Bez regresji krytycznych. F-01 (cache agenta WebDAV na SSD, wysokie) otwarte → S002. F-02 (token widoczny w `mount`) zaakceptowane w prototypie.
- Dokumentacja: `docs/threat-model.md`, `docs/uzytkowanie.md`. Szkice S002 (integracja bez cache) i S003 (ochrona katalogu z Findera, zgłoszenie właściciela).

### Start projektu

- `git init` (gałąź `main`), artefakty CDD (`CLAUDE.md`, `README.md`, `CHANGELOG.md`, `MANIFEST.md`, `specs/SZABLON.md`), `.gitignore`. Pusty katalog `tests/` usunięty w S001: testy Go leżą przy pakietach.
