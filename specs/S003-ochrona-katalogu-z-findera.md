# S003 — Ochrona katalogu z poziomu Findera

- **Status:** szkic — zgłoszenie właściciela, wymaga sesji specyfikacyjnej
- **Data:** 2026-11-22
- **Powiązane:** S001, S002

## 1. TL;DR

Użytkownik zaznacza katalog w Finderze i z menu kontekstowego wybiera „Chroń kartą SD (xordock)”. Katalog trafia do sejfu, a jego miejsce zajmuje zamontowany chroniony katalog. Bez terminala.

## 2. Abstract

Pytanie właściciela (2026-11-22): „jak mogę zaznaczyć katalog i wybrać szyfrowanie na karcie SD jako kluczu, czy jest tylko opcja z cmd?”. W S001 jedyną drogą jest CLI (`xordock init/mount`), a sejf zaczyna jako pusty katalog.

Przeniesienie **istniejącego** katalogu ma ważny skutek: oryginalne jawne pliki zostają w wolnych blokach SSD po usunięciu. Na APFS/SSD nie da się ich pewnie nadpisać (copy-on-write, TRIM). Ochronę zapewnia tylko FileVault. Specyfikacja musi powiedzieć to użytkownikowi wprost, zanim zgodzi się na import (G-06, FR-24).

## 3. Objective

- **Cele:** akcja Findera (Quick Action / Usługa) „Chroń kartą SD”; import katalogu do nowego albo istniejącego sejfu; montowanie w tym samym miejscu; akcje „Odmontuj” i „Zamontuj” bez terminala; aplikacja w pasku menu pokazująca stan sejfu i karty.
- **Poza zakresem:** [DO USTALENIA]
- **Kryteria sukcesu:** [DO USTALENIA]

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `xordock import` (CLI) | kod | do utworzenia |
| Quick Action Findera (Automator `.workflow` wywołujący CLI) | kod | do utworzenia |
| Aplikacja w pasku menu (montowanie w tle, stan karty) | kod | [DO USTALENIA] (zależy od S002: FSKit daje to naturalnie) |

## 5. Functional Domain Changes

[DO USTALENIA]. Pytania do sesji:

1. Import: przenieść (oryginał usunięty, ale resztki na SSD) czy skopiować (oryginał zostaje, użytkownik usuwa sam)?
2. Jeden sejf na kartę z wieloma katalogami czy osobny sejf na każdy katalog?
3. Czy chroniony katalog ma się montować w miejscu oryginału (np. `~/Documents/Umowy`), czy w `~/xordock/<nazwa>`?
4. Czy montowanie po włożeniu karty ma być automatyczne?
5. Kolejność względem S002: menu w Finderze można zrobić od razu na WebDAV (Automator, bez Xcode), aplikację w pasku menu lepiej po decyzji FSKit.

## 6–9.

Do uzupełnienia na sesji specyfikacyjnej.
