# S010 — Samouczek: wprowadzenie i prowadzenie przez akcje

- **Status:** dostarczona (G1–G3, G5); G4 i przejście samouczka do sprawdzenia przez właściciela
- **Data:** 2026-11-22
- **Powiązane:** S007, S008, S009

## 1. TL;DR

Przy każdym uruchomieniu aplikacja pyta, czy włączyć samouczek. Opcję „Nie pytaj przy starcie” można potem odwrócić w Ustawieniach.

Samouczek ma dwie części:

- 6 ekranów z przyciskiem „Dalej”, które tłumaczą problem i rozwiązanie;
- prowadzenie po prawdziwym sejfie: dymek wskazuje przycisk, użytkownik wykonuje akcję (załóż sejf, otwórz, odmontuj, backup), a samouczek sam przechodzi dalej, gdy stan sejfu się zmieni.

## 2. Abstract

Po S009 pulpit jest czytelny, ale nowa osoba nie wie, po co są dwie karty, iCloud i backup, ani w jakiej kolejności działać.

Decyzje właściciela (2026-11-22):

- **Pytanie przy starcie:** przy każdym uruchomieniu, z polem „Nie pytaj przy starcie”; zmiana w Ustawieniach.
- **Wprowadzenie:** proste, 5–6 ekranów, bez matematyki.
- **Dymki na prawdziwym sejfie:** akcje prowadzą przez samouczek.

## 3. Objective

- **Cele:**
  - T-01 **Pytanie przy starcie.** Okno otwiera się z arkuszem „Samouczek?”: [Uruchom samouczek] [Nie teraz] i pole „Nie pytaj przy starcie”. Ustawienie trzyma `UserDefaults`.
  - T-02 **Ustawienia** (⌘, oraz pozycja w menu kłódki): przełącznik „Pytaj o samouczek przy starcie” i przycisk „Uruchom samouczek teraz”.
  - T-03 **Wprowadzenie:** 6 ekranów (problem, pomysł, gdzie leżą połówki, kopie, zasady, uczciwe granice ochrony) z ikoną, tytułem i krótkim tekstem; przyciski Wstecz, Dalej, Pomiń wprowadzenie.
  - T-04 **Prowadzenie.** Kroki zakotwiczone dymkiem przy elemencie interfejsu:

    | Krok | Zakończenie |
    | --- | --- |
    | załóż sejf | automatycznie, gdy sejf istnieje; pomijany, jeśli już jest |
    | status | „Dalej” |
    | kafelki | „Dalej” |
    | otwórz sejf | automatycznie, gdy zamontowany |
    | wrzuć pliki w Finderze | „Dalej” |
    | odmontuj | automatycznie, gdy odmontowany |
    | zrób backup | automatycznie, gdy pojawi się nowy punkt READY |
    | odzyskiwanie | „Dalej” |
    | pasek menu i ustawienia | „Zakończ” |
  - T-05 **Pasek samouczka** na dole okna: numer kroku, „Pokaż dymek”, Wstecz, Dalej/Pomiń krok, Zakończ. Zamknięty dymek nie przerywa samouczka.
  - T-06 Wskazówki samouczka w arkuszach „Nowy sejf” i „Backup” (wybór nośników).
- **Poza zakresem:** filmy i animacje, tłumaczenia, nauka odzyskiwania na prawdziwych danych (krok tylko tłumaczy i wskazuje „Weryfikuj”).
- **Kryteria sukcesu:** bramki z §7 i przejście samouczka przez właściciela.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `macos/Sources/XordockCore/Tutorial.swift` | kod: treść ekranów, kroki, warunki zakończenia, maszyna stanów | do utworzenia |
| `macos/Sources/XordockUI/Tutorial.swift` | kod: ekrany wprowadzenia, dymek, pasek samouczka | do utworzenia |
| `macos/Sources/XordockApp/*` | kod: pytanie przy starcie, Ustawienia, kotwice dymków | do zmiany |

## 5. Functional Domain Changes

- Maszyna stanów `Tour` jest czysta (Core):
  - `start(state)` pomija krok „załóż sejf”, gdy sejf już jest;
  - `update(state)` przesuwa automatycznie po spełnieniu warunku;
  - `next`, `back`, `finish`.
- Krok „zrób backup” zapamiętuje identyfikator ostatniego punktu w chwili wejścia i kończy się, gdy pojawi się inny.
- Samouczek działa na sejfie wybranym w oknie. Gdy wybór się zmieni, warunki dotyczą nowego wyboru.
- Samouczek niczego nie wykonuje sam. Akcje (zapis na nośniki) robi użytkownik, tymi samymi przyciskami i arkuszami co zwykle (S004 §7).
- Ustawienia: `askTutorialAtLaunch` (domyślnie `true`).

## 6. Critical Path

1. Core z testami.
2. Widoki wprowadzenia, dymku i paska.
3. Kotwice w pulpicie i na liście, pytanie przy starcie, Ustawienia.
4. Rendery ekranów do PNG, budowanie `.app`.

## 7. Control Gates

| # | Bramka | Warunek | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | Maszyna stanów | pomijanie „załóż sejf”, automatyczne przejścia (montowanie, odmontowanie, nowy punkt), Wstecz/Dalej/Zakończ | `swift test` | commit |
| G2 | Treść | 6 ekranów; tytuł ≤ 6 słów, tekst ≤ 320 znaków; każdy krok ma tytuł i tekst ≤ 220 znaków | `swift test` | commit |
| G3 | **Negatywna:** samouczek nie działa sam | Core nie wywołuje poleceń; w kodzie samouczka brak `Commands.` | test źródeł | commit |
| G4 | Ustawienie | „Nie pytaj przy starcie” zapisane i odwracalne w Ustawieniach | przegląd + E2E | wydanie |
| G5 | Wygląd | rendery ekranów wprowadzenia i dymku (jasny/ciemny) bez uciętych tekstów | przegląd | commit |

## 8. Expected Outcome

Nowa osoba rozumie sens podziału i kopii, a pierwszy sejf z backupem zakłada, idąc za dymkami. Wycofanie: wyłączenie pytania w Ustawieniach.

## 9. Additional Information

- **Kotwice dymków:** natywne `popover` SwiftUI, które działają w kolumnie listy, w pulpicie i w pasku narzędzi. Kliknięcie poza dymkiem go zamyka, a samouczek trwa (pasek ma „Pokaż dymek”).
- **Pasek menu:** dymka nie da się zakotwiczyć przy ikonie w pasku menu, więc ostatni krok opisuje go słowami.

## Realizacja (2026-11-22)

- **Core:** `XordockCore/Tutorial.swift`: `Intro.pages` (6 ekranów; zasady jako lista), `TourStep` (9 kroków z kotwicą, tytułem, tekstem i flagą „automatyczny”), maszyna `Tour`.
- **UI:** `XordockUI/Tutorial.swift`:
  - `TourController` (`@Observable`, wspólny dla okna i arkuszy);
  - `TourBubble`, modyfikator `.tourAnchor(…)` (natywny `popover`), `TourBar`, `TourHint`, `IntroView`, `StartPrompt`.
  - Zamknięcie dymku kliknięciem obok tylko go ukrywa („Pokaż dymek” w pasku).
- **Kotwice:**
  - „Nowy sejf…” na liście;
  - karta statusu, kafelki;
  - „Otwórz sejf”, „Odmontuj”, „Backup…”, „Odzyskiwanie…”. Przycisk główny dostaje kotwicę „mount” lub „backup” zależnie od zalecenia, więc dymek trafia na widoczny przycisk.
- **Start:**
  - ikona w pasku menu (rysowana od startu) otwiera okno, gdy `askTutorialAtLaunch` = true; okno pokazuje `StartPrompt`;
  - menu kłódki: „Samouczek…” i „Ustawienia…” (`SettingsLink`, ⌘,);
  - Ustawienia: przełącznik i „Uruchom samouczek teraz”.
- **Testy:** `tourFlow` (G1), `tutorialTexts` (G2), `tutorialRunsNoCommands` (G3). Razem 16 testów Swift.
- **G5:** rendery 6 ekranów, dymków i pytania w obu trybach (`XordockPreview`). Przykłady w `docs/ui/`. Po przeglądzie ekran „Pięć zasad” zamieniony na listę.
