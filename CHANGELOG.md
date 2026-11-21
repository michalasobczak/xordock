# CHANGELOG

Chronologiczny zapis tego, co faktycznie dostarczono. Wpisy są uporządkowane według dat, najnowsze na górze.

## 2026-11-22

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
