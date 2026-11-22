# S005 — Poprawki z analizy bezpieczeństwa rundy S004

- **Status:** dostarczona
- **Data:** 2026-11-22
- **Powiązane:** S004, `docs/bezpieczenstwo/2026-11-22-runda-S004.md` (R4-01, R4-02)

## 1. TL;DR

Dwie luki znalezione w analizie po S004 zostały poprawione: `backup prune` nie usuwa starszych punktów, dopóki najnowszy nie jest kompletny, a indeks ze ścieżkami spoza drzewa jest odrzucany. Dowód: testy `TestPruneRefusesIncompleteNewest` i `TestIndexPathsValidated`.

## 2. Abstract

Po implementacji S004 przejrzano kod pod kątem regresji i luk. `PruneBackup` sprawdzał tylko, czy da się złożyć indeks najnowszego punktu, a potem usuwał starsze punkty i ich udziały. Gdyby któryś blok najnowszego punktu był uszkodzony, użytkownik straciłby jedyną dobrą kopię, wbrew B-04 i zakazowi „nie wolno automatycznie usuwać ostatniego backupu”.

Indeks wczytany z udziałów nie był walidowany. Uszkodzony albo celowo podmieniony udział (plastyczność XOR, F-03) mógł zawierać ścieżkę z `..`, a `export` i `recover export` zapisałyby plik poza katalogiem docelowym. Integralność wobec przeciwnika jest poza zakresem MVP, ale zapis poza wskazanym katalogiem to szkoda dla hosta, a poprawka jest tania.

## 3. Objective

- **Cele:** prune usuwa cokolwiek dopiero po sprawdzeniu obu udziałów każdego obiektu najnowszego punktu; indeks z niekanoniczną ścieżką albo wpisem bez katalogu nadrzędnego to `ErrCorrupt`; eksport odmawia ścieżki wychodzącej poza katalog.
- **Poza zakresem:** uwierzytelnianie udziałów (F-03, osobna decyzja kryptograficzna).
- **Kryteria sukcesu:** oba testy zielone, pełny `go test -race ./...` zielony.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `internal/vault/backup.go` (`PruneBackup`) | kod | zmieniony |
| `internal/vault/vault.go` (`Index.validate`, `loadIndexFrom`) | kod | zmieniony |
| `internal/vault/recovery.go` (`ExportTo`) | kod | zmieniony |
| `internal/vault/backup_test.go` | test | zmieniony |

## 5. Functional Domain Changes

- `PruneBackup`: przed usunięciem czegokolwiek dekoduje udział B (karta zapasowa) i A (SSD) każdego obiektu najnowszego punktu; błąd → nic nie jest usuwane.
- `loadIndexFrom` wywołuje `Index.validate`: każda ścieżka równa `Clean(p)`, zaczyna się od `/`, ma katalog nadrzędny będący katalogiem.
- `ExportTo` dodatkowo sprawdza, że ścieżka docelowa leży w katalogu eksportu (obrona w głąb).
- Wpływ na dane: brak; poprawne indeksy zawsze spełniały te warunki (operacje używają `Clean`).

## 6. Critical Path

Testy odtwarzające (czerwone) → poprawki → testy zielone → wpis w CHANGELOG.

## 7. Control Gates

| # | Bramka | Warunek zaliczenia | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | Prune nie traci jedynej dobrej kopii | przy uszkodzonym bloku najnowszego punktu prune zwraca błąd, oba punkty zostają | `TestPruneRefusesIncompleteNewest` | commit |
| G2 | Brak ścieżek spoza drzewa | indeks z `..`, ścieżką względną, `//`, końcowym `/` albo bez rodzica odrzucony | `TestIndexPathsValidated` | commit |

## 8. Expected Outcome

Retencja jest bezpieczna także przy uszkodzonym nośniku; eksport nie wychodzi poza katalog docelowy. Wycofanie: rewert commitu (bez zmian formatu).

## 9. Additional Information

Znalezione w analizie bezpieczeństwa rundy S004 (wymóg CLAUDE.md: analiza po każdej rundzie, znaleziska poprawiane jak zwykła zmiana).
