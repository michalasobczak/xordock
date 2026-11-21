# Model zagrożeń xordocka

Stan na 2026-11-22 (prototyp S001). Źródło: S001 §5.7, decyzje §9.8, analiza `docs/bezpieczenstwo/2026-11-22-runda-S001.md`.

## Co chronimy

Treść plików i ich nazwy (nazwy i struktura katalogów są w indeksie dzielonym XOR).

## Przed kim

| Przeciwnik | Chroniony? |
|---|---|
| Ma pełną kopię udziału A (SSD, kopia Time Machine, skradziony dysk), także wiele historycznych kopii | **Tak.** W prototypie obliczeniowo (CSPRNG); bezwarunkowo dopiero po IT-01–IT-04. |
| Ma tylko kartę SD (udział B) | **Tak.** B to same maski. |
| Ma A i B | **Nie** (poza zakresem). |
| Kontroluje Maca, gdy sejf jest zamontowany (malware, inne konto lokalne) | **Nie** (poza zakresem; F-02). |
| Modyfikuje jeden udział, żeby zmienić treść | **Nie wykrywamy** (F-03, R-04). Przypadkowe uszkodzenia wykrywamy. |
| Ma SSD wyłączonego Maca z FileVault | Chronią go FileVault i xordock. Bez FileVault cache macOS (F-01) może zawierać jawne fragmenty, dlatego `mount` wymaga FileVault. |

## Czego nie ukrywamy

Rozmiarów i liczby obiektów, czasów zapisu, nazwy sejfu, faktu używania xordocka (R-06, F-05).

## Granice systemu (FR-25, IT-03)

Jawne dane opuszczają sejf w czasie pracy: RAM aplikacji, cache agenta WebDAV (F-01), pliki tymczasowe i autozapis aplikacji, miniatury QuickLook, historia ostatnich dokumentów. Szczegóły i zalecenia: `docs/uzytkowanie.md`.
