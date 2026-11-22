# S008 — Kopia udziału A w iCloud Drive

- **Status:** dostarczona (G1–G6 testy, E2E na pendrive'ach i iCloud); G7 (aplikacja) do testu właściciela
- **Data:** 2026-11-22
- **Powiązane:** S004 (§5.5–5.6, B-01–B-05, R-02), S006, S007

## 1. TL;DR

Backup zapisuje odtąd jeden punkt odzyskiwania w dwóch miejscach:

- udziały B migawki na kartę zapasową;
- udziały A tej samej migawki do `iCloud Drive/xordock/<sejf>`.

Po utracie SSD dane odzyskuje się z kopii A w iCloud razem z kartą zapasową albo z kartą roboczą. Potwierdza to test, który usuwa katalog A i odzyskuje bajty z kopii.

## 2. Abstract

Po S004 udział B ma dwie kopie (karta robocza i zapasowa), a udział A tylko jedną, na SSD. Utrata SSD (awaria, kradzież Maca, reinstalacja) oznacza więc utratę danych. Karta zapasowa nic tu nie pomoże, bo odzyskiwanie wymaga A.

Właściciel zauważył asymetrię: dwa klucze na dwóch nośnikach wymagają też dwóch kopii A. Sprawdzono, że iCloud na macOS nie kopiuje `~/Library/Application Support`. Time Machine kopiuje, ale jego dysk bywa odłączony.

Udziały A można bezpiecznie trzymać poza Makiem pod warunkiem, że to samo miejsce nigdy nie dostanie żadnego B. Ochrona jest wtedy praktyczna: konto iCloud plus dowolna karta B dają komplet.

Decyzje właściciela (2026-11-22):

- **Kiedy:** razem z backupem. Jeden punkt, para A + B z tej samej migawki.
- **Retencja:** jak na karcie zapasowej. Przycinanie punktów obejmuje oba miejsca.
- **Miejsce:** iCloud Drive na sztywno: `~/Library/Mobile Documents/com~apple~CloudDocs/xordock/<sejf>`.
- **Advanced Data Protection:** jednorazowe ostrzeżenie w aplikacji z potwierdzeniem. CLI wypisuje ostrzeżenie przy każdym backupie z kopią A.

## 3. Objective

- **Cele:**
  - K-01 `backup` kopiuje udziały A migawki do iCloud Drive pod tym samym identyfikatorem punktu co kopia B. Kopia A dostaje własny nagłówek (`side:A`, `role:backup`, `media`) i rekord `points/<id>` READY po weryfikacji.
  - K-02 Przy braku iCloud Drive (katalog nie istnieje) backup B działa jak dotąd, a CLI i aplikacja wyraźnie mówią, że kopii A nie ma. `--no-icloud` świadomie ją pomija.
  - K-03 `backup prune` przycina też kopię A (zostaje najnowszy punkt, po weryfikacji kompletności).
  - K-04 Odzyskiwanie bez SSD: `recover --from-icloud --spare <karta zapasowa albo robocza>` (list, verify, export, restore). Punkty pochodzą z rekordów kopii, które mają rolę backup.
  - K-05 Aplikacja:
    - pokazuje stan kopii A (ostatni punkt);
    - ostrzega jednorazowo przed pierwszą kopią do iCloud (ADP, 2FA);
    - pokazuje kopie z iCloud, dla których na tym Macu nie ma udziału A, z przyciskiem „Odzyskiwanie…”.
- **Poza zakresem:**
  - wykrywanie, czy ADP jest włączone (brak publicznego API);
  - inne chmury;
  - kopia A bez backupu B;
  - szyfrowanie kopii A dodatkowym hasłem.
- **Kryteria sukcesu:** bramki z §7.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `internal/vault/backup.go` | kod: `BackupPair`, uogólnione kopiowanie strony i przycinanie | do zmiany |
| `internal/vault/recovery.go` | kod: odzyskiwanie z kopii A | do zmiany |
| `cmd/xordock/backup.go`, `app.go`, `main.go` | kod CLI | do zmiany |
| `macos/Sources/*` | aplikacja | do zmiany |
| `docs/recovery.md`, `docs/format-v1.md` | dokumentacja | do zmiany |

## 5. Functional Domain Changes

- **Układ kopii A:** taki sam jak karty zapasowej (`xordock.json`, `obj/xx/<oid>`, `points/<id>`), ale nagłówek ma `side:"A"`. Udziały są kopiowane bajt w bajt, więc maski się nie zmieniają.
- **Kolejność:**
  1. Sprawdzenie miejsca na obu celach.
  2. Przypięcie migawki dla obu nośników (dwa `PointRef` z tym samym `Point`).
  3. Kopia B, weryfikacja i punkt READY na karcie.
  4. Kopia A, weryfikacja i punkt READY w iCloud.
  5. Flagi Ready w indeksie.

  Przerwanie przed krokiem 4 zostawia poprzednie punkty A nietknięte. Błąd kopii A po READY karty nie unieważnia punktu B; komunikat mówi, że kopia A się nie udała.
- **Reguły bezpieczeństwa:**
  - Cel kopii A nie może leżeć na urządzeniu karty roboczej ani zapasowej, ani w katalogu udziału A czy B.
  - Do katalogu kopii A nie trafia żaden udział B: sprawdza to `decodeShare` strony A przed zapisem.
- **Odzyskiwanie:** `OpenRecovery(a, b)` przyjmuje jako A katalog żywy albo kopię, a jako B kartę zapasową albo roboczą. Przynajmniej jedna strona musi być kopią z rekordami punktów. Kopia nie jest blokowana ani zmieniana.
- **iCloud:**
  - Pliki kopii mogą zostać wyrzucone z dysku (optymalizacja miejsca). Odczyt przy odzyskiwaniu je pobiera.
  - Zapis atomowy (tmp + rename) nie zostawia w iCloud połówek plików jako gotowych obiektów, bo nazwa docelowa pojawia się dopiero po rename.

## 6. Critical Path

1. Silnik: `BackupPair`, przycinanie kopii A, odzyskiwanie z kopii, wraz z testami.
2. CLI: domyślna kopia do iCloud przy `backup`, `--no-icloud`, `recover --from-icloud`, pole `aCopy` i lista `orphanCopies` w `vaults`.
3. Aplikacja.
4. E2E: backup na pendrive + iCloud, potem odzyskanie z kopii przy przemianowanym katalogu A.

## 7. Control Gates

| # | Bramka | Warunek | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | Utrata SSD | po usunięciu katalogu A dane punktu odzyskane z kopii A + karty zapasowej, bajt w bajt | `TestRecoverAfterLosingSSD` | commit |
| G2 | Utrata SSD i karty zapasowej | kopia A + karta robocza odtwarza punkt | ten sam test | commit |
| G3 | **Negatywna:** brak B w kopii A | każdy plik w kopii A to udział strony A albo nagłówek/rekord; brak jawnej treści | `TestACopyHoldsNoBNoPlaintext` | commit |
| G4 | **Negatywna:** cel kopii A ≠ karta | odmowa, gdy cel leży w katalogu B lub na karcie zapasowej | test | commit |
| G5 | Przerwanie kopii A | poprzedni punkt A dalej odzyskiwalny, punkt B READY | `TestInterruptedACopy` | commit |
| G6 | Retencja | `prune` zostawia na obu celach tylko najnowszy punkt | test | commit |
| G7 | Aplikacja | ostrzeżenie ADP raz, stan kopii A, odzyskiwanie bez A | E2E | wydanie |

## 8. Expected Outcome

- Każdy z trzech nośników (SSD, karta robocza, karta zapasowa) można stracić pojedynczo bez utraty danych z ostatniego punktu.
- Koszt: miejsce w iCloud ≈ rozmiar danych migawki.
- Wycofanie: `--no-icloud`; usunięcie katalogu `iCloud Drive/xordock` nie dotyka sejfu.

## 9. Additional Information

- **Ryzyko:** konto iCloud staje się nośnikiem udziału A. Kto przejmie konto, potrzebuje już tylko jednej karty. Łagodzi to 2FA, ADP oraz trzymanie kart osobno.
- **Odrzucone:**
  - przeniesienie żywego A do iCloud Drive: wyrzucanie plików z dysku, konflikty synchronizacji, brak pracy offline;
  - montowanie jawnego woluminu w Dokumentach: jawna treść w chmurze.

## Realizacja (2026-11-22)

- **Silnik:**
  - `Backup` i `BackupPair` korzystają z jednej ścieżki `backup` → `copySide` dla strony A albo B.
  - Haki testowe: `backup-*` dla B, `acopy-*` dla A.
  - `prepareCopy` odrzuca cele leżące w katalogach udziałów i na urządzeniu karty roboczej. `BackupPair` odrzuca kopię A na urządzeniu karty zapasowej. Pełna kontrola fizycznego nośnika należy do S006.
  - `PruneACopy`.
  - `OpenRecovery` przyjmuje kopię A i kartę roboczą. Gdy obie strony są kopiami, punkty to część wspólna. Kopie nie są blokowane.
- **CLI:**
  - `backup` domyślnie robi kopię A, gdy istnieje iCloud Drive (`--no-icloud`); `backup prune` przycina oba cele;
  - `recover`/`points --from-icloud`;
  - `status` pokazuje kopię A;
  - `vaults`: pola `icloud`, `aCopy` i lista `orphanCopies` (kopie bez udziału A na Macu).
- **Aplikacja:**
  - ostrzeżenie ADP raz (`UserDefaults adpWarningAccepted`);
  - wiersz „Kopia udziału A (iCloud)” i ostrzeżenie, gdy jej brak;
  - sekcja „Kopie A w iCloud bez SSD” z odzyskiwaniem;
  - przełącznik „Udział A z kopii w iCloud” w odzyskiwaniu.
- **Testy:**
  - `TestRecoverAfterLosingSSD` (G1, G2), `TestACopyHoldsNoBNoPlaintext` (G3), `TestACopyRejectsCardLocations` (G4), `TestInterruptedACopy` (G5, 3 etapy), `TestPruneBothCopies` (G6), `TestACopyInState`;
  - Swift: `aCopyStateAndArgs`.
- **E2E (2026-11-22):**
  - sejf tymczasowy: A w katalogu tymczasowym, B na pendrivie `xor_primary`, zapas na `xor_backup`, kopia A w prawdziwym iCloud Drive;
  - plik 3 MB z markerem wgrany przez Findera, migawka i backup pary;
  - katalog A przemianowany („utrata SSD”), potem `recover --from-icloud verify/export` z kartą zapasową i z roboczą: `cmp` identyczne;
  - brak markera w kopii A, na kartach i w starym A;
  - `vaults` pokazał kopię jako osieroconą;
  - wszystko usunięte z kart i z iCloud Drive.
