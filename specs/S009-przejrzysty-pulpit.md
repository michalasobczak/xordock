# S009 — Przejrzysty pulpit sejfu

- **Status:** dostarczona (G1–G5); akceptacja wyglądu przez właściciela w działającej aplikacji
- **Data:** 2026-11-22
- **Powiązane:** S007, S008

## 1. TL;DR

Widok sejfu w aplikacji zmienia się z formularza pełnego tekstu w pulpit:

- jeden duży status z jednym głównym przyciskiem („co teraz zrobić”);
- cztery kafelki nośników (SSD/A, karta robocza/B, karta zapasowa/kopia B, iCloud/kopia A) z ✓/⚠︎ i datą;
- szczegóły techniczne zwinięte.

Wynik potwierdzają testy reguł statusu i rendery widoku do PNG.

## 2. Abstract

Właściciel po pierwszych testach aplikacji (S007, S008) ocenił okno jako mało przejrzyste:

- za dużo tekstu;
- nie wiadomo, co zrobić dalej;
- stan nośników jest niejasny.

Wybrał układ „pulpit z kafelkami” (2026-11-22).

## 3. Objective

- **Cele:**
  - P-01 **Status z jednym zaleceniem.** Pierwsza pasująca reguła (kolejność priorytetu):
    1. błąd sejfu;
    2. brak karty roboczej;
    3. brak backupu;
    4. backup starszy niż 7 dni;
    5. brak kopii A w iCloud;
    6. sejf otwarty;
    7. wszystko w porządku.

    Każdy stan ma tytuł do 5 słów, jedno zdanie wyjaśnienia i najwyżej jeden przycisk główny.
  - P-02 **Cztery kafelki:** nazwa nośnika, rola jednym słowem, stan ✓/⚠︎/–, data ostatniej kopii lub nazwa woluminu.
  - P-03 **Akcje drugorzędne** w jednym rzędzie małych przycisków. „Więcej” mieści fsck, log i szczegóły.
  - P-04 **Szczegóły techniczne** (ścieżki, maski, commit, migawki) w zwijanej sekcji, domyślnie zwiniętej.
  - P-05 Na liście sejfów kropka koloru stanu zamiast opisu.
  - P-06 Teksty bezpieczeństwa (CG-14, ADP, eksport jawny) zostają, ale w miejscu akcji, której dotyczą.
- **Poza zakresem:** zmiany CLI i formatu, arkusze backupu i odzyskiwania (poza skróceniem tekstów).
- **Kryteria sukcesu:** bramki z §7 i akceptacja właściciela.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `macos/Sources/XordockCore/Health.swift` | kod: reguły statusu i kafelków | do utworzenia |
| `macos/Sources/XordockUI/Dashboard.swift` | kod: widok pulpitu (biblioteka, renderowalna) | do utworzenia |
| `macos/Sources/XordockPreview/main.swift` | narzędzie: render pulpitu do PNG dla przykładowych stanów | do utworzenia |
| `macos/Sources/XordockApp/*` | kod | do zmiany |

## 5. Functional Domain Changes

- Stan pulpitu liczy czysta funkcja `VaultHealth.evaluate(vault, icloud:)`. Widok nie zawiera logiki decyzyjnej.
- Kafelek karty zapasowej pokazuje datę ostatniego punktu READY i informację, czy karta jest teraz podłączona.
- Kafelek iCloud: „wyłączony”, gdy iCloud Drive niedostępny.

## 6. Critical Path

1. `Health.swift` z testami.
2. `XordockUI` z pulpitem.
3. Podpięcie w aplikacji.
4. Rendery PNG i przegląd.
5. Budowanie `.app`.

## 7. Control Gates

| # | Bramka | Warunek | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | Reguły | każdy z 7 stanów i priorytety | `swift test` | commit |
| G2 | Kafelki | stany ✓/⚠︎/– dla kart podłączonych i niepodłączonych, iCloud wyłączony | `swift test` | commit |
| G3 | Zwięzłość | tytuł ≤ 5 słów, wyjaśnienie ≤ 1 zdanie (≤ 120 znaków) dla każdego stanu | `swift test` | commit |
| G4 | Wygląd | rendery PNG 4 stanów obejrzane: jasny i ciemny tryb, brak uciętych tekstów | przegląd | commit |
| G5 | Bez regresji | testy S007/S008 zielone, G4/G5 S007 (bez powłoki, bez dostępu do udziałów) | `swift test` | commit |

## 8. Expected Outcome

Po otwarciu okna od razu widać, czy jest dobrze i co zrobić. Wycofanie: przywrócenie poprzedniego `VaultDetail`.

## 9. Additional Information

Brak dostępu do ekranu właściciela (uprawnienia macOS), więc wygląd sprawdzamy renderami `ImageRenderer`.

## Realizacja (2026-11-22)

- `XordockCore/Health.swift`: `VaultHealth.evaluate` (7 stanów, priorytety P-01), `VaultInfo.tiles(icloud:)` (P-02), daty po polsku niezależnie od języka systemu.
- `XordockUI` (nowa biblioteka):
  - `Dashboard`: status z przyciskiem głównym, 4 kafelki, rząd akcji, zwinięte szczegóły;
  - `HealthDot`;
  - własne style przycisków (czysty SwiftUI, renderowalne);
  - akcja będąca przyciskiem głównym nie powtarza się w rzędzie.
- Aplikacja:
  - pulpit zastępuje formularz `VaultDetail`; kropki stanu na liście; menu w pasku pokazuje tytuł statusu;
  - komunikaty po akcjach to jedno zdanie, a pełne wyjście CLI jest pod „Szczegóły”;
  - krótsze banery.
- `XordockPreview`: `swift run XordockPreview <katalog>` renderuje 4 stany × jasny/ciemny tryb do PNG. Przykłady w `docs/ui/`. To narzędzie deweloperskie, nie trafia do `.app`.
- **Bramki:**
  - G1–G3: testy `healthPriorities`, `tilesStates`, `healthTextsAreShort`;
  - G4: rendery obejrzane (poprawiono dublowany przycisk „Otwórz sejf”, angielskie daty, wyrównanie kafelka „Ten Mac”);
  - G5: 13 testów Swift zielonych, w tym zakaz powłoki i dostępu do udziałów, który obejmuje nowe źródła.
