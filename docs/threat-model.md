# Model zagrożeń xordocka

Stan na 2026-11-22 (S004). Źródło: S004 §5.7 i §9, analizy `docs/bezpieczenstwo/2026-11-22-runda-S001.md` i `2026-11-22-runda-S004.md`.

## Co chronimy

Treść plików i ich nazwy (nazwy i struktura katalogów są w indeksie dzielonym XOR).

## Przed kim

| Przeciwnik | Chroniony? |
|---|---|
| Ma pełną kopię udziału A (SSD, kopia Time Machine, skradziony dysk), także wiele historycznych kopii | **Tak**, praktycznie (CSPRNG macOS `SecRandomCopyBytes`). To **nie** jest gwarancja informacyjno-teoretyczna ani odporność na P = NP (G-09, CG-14). |
| Ma tylko kartę roboczą albo tylko kartę zapasową (udział B) | **Tak.** B to same maski. |
| Ma A i którąkolwiek kartę B (roboczą albo zapasową) | **Nie** (poza zakresem). Karta zapasowa to kopia tego samego udziału B, nie osobny klucz (B-02); karta robocza trzyma też maski migawek (R4-06). |
| Kontroluje Maca, gdy sejf jest zamontowany (malware, administrator) | **Nie** (poza zakresem, I-05). Inne konta lokalne nie mają dostępu do wolumenu (macFUSE bez `allow_other`). |
| Modyfikuje jeden udział, żeby zmienić treść | **Nie wykrywamy** (F-03, R-04). Przypadkowe uszkodzenia wykrywamy. |
| Ma SSD wyłączonego Maca z FileVault | Chronią go FileVault i xordock. Bez FileVault pliki tymczasowe i cache aplikacji mogą zawierać jawne fragmenty (FR-25). |

## Czego nie ukrywamy

Rozmiarów i liczby obiektów, czasów zapisu, nazwy sejfu (także w punkcie montowania), liczby i dat punktów odzyskiwania, faktu używania xordocka (D-04, F-05).

## Granice systemu (FR-25, IT-03)

Jawne dane opuszczają sejf w czasie pracy: RAM aplikacji i rozszerzenia FSKit macFUSE, pliki tymczasowe i autozapis aplikacji, miniatury QuickLook, indeks Spotlight, crash dumpy, Time Machine i iCloud dla kopii zrobionych przez aplikacje, historia ostatnich dokumentów (I-04, I-06). Szczegóły i zalecenia: `docs/uzytkowanie.md`.
