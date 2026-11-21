# Manifest

Zasady inżynierskie xordocka. Obowiązują przy każdej specyfikacji w `specs/`. Specyfikacja, która łamie którąś z zasad, musi to powiedzieć wprost i uzasadnić w sekcji „Additional Information”.

Bazą są Engineering essentials z https://workwithrondo.com/#builders (E1–E6). Konkretne decyzje projektu (stack, środowiska, deploy) dopisujemy tutaj, gdy zapadną.

## E1. Projekt i technologia

**M1. Prostota.** Sprawdzone wzorce, jasne odpowiedzialności, tylko potrzebne abstrakcje.
**M2. Bez duplikacji.** Wspólna logika w jednym miejscu; wyjątek uzasadniamy w specyfikacji.
**M3. Stabilne technologie i minimum zależności.** Każda biblioteka i każda dodatkowa usługa (kolejka, NoSQL itp.) wymaga uzasadnienia.
**Stack:** silnik sejfu w Go 1.27 (tylko biblioteka standardowa). macOS 27, Apple Silicon. Integracja z Finderem: natywne rozszerzenie FSKit (S002, Xcode 27.2); WebDAV i NFS odrzucone.

**M24. Jawne dane tylko w RAM.** Kod xordocka nigdy nie zapisuje jawnej treści ani nazw plików na dysk ani do logów. Bufory z treścią czyścimy (`split.Wipe`) po użyciu.
**M25. Uczciwe deklaracje.** Każdy komunikat i dokument o bezpieczeństwie mówi, z jakiego źródła masek korzystamy; CSPRNG nigdy nie jest przedstawiany jako odporny na P = NP.
**M26. Jeden udział, jedna suma.** Suma kontrolna obejmuje tylko własny udział. Nie zapisujemy na jednym nośniku niczego, co wyliczono z treści drugiego.

## E2. Kontrakty i dane

**M4. Wersjonowane kontrakty API** z testami zgodności.
**M5. Idempotencja** operacji powtarzanych.
**M6. Integralność danych:** transakcje, ograniczenia, wersjonowane i testowane migracje.
**M7. Wejście jest niezaufane:** walidacja typu, formatu, rozmiaru, zakresu i uprawnień.

## E3. Dostęp i audyt

**M8. Weryfikacja tożsamości:** bezpieczne hasła, 2FA dla administratorów, brak enumeracji kont.
**M9. Najmniejsze uprawnienia** sprawdzane przy każdym dostępie do zasobu.
**M10. Audyt zmian:** kto, kiedy, co (bez sekretów); zasada czterech oczu tam, gdzie wymagana.

## E4. Ochrona systemu

**M11. Sekrety poza kodem, frontendem i logami;** osobne per środowisko, z rotacją.
**M12. HTTPS, nagłówki bezpieczeństwa, szyfrowanie danych w spoczynku** według wrażliwości.
**M13. Izolacja wgrywanych plików.**
**M14. Ograniczony dostęp do środowisk:** bez publicznego SSH, tylko potrzebne porty.

## E5. Wdrożenie i odtwarzanie

**M15. Kod, specyfikacje, migracje i konfiguracja (bez sekretów) w Git;** regularny push do prywatnego zdalnego repozytorium.
**M16. Kontenery** (np. Docker Compose).
**M17. Deploy przez CI:** build, testy, bramki kontrolne, wdrożenie; bez omijania błędów.
**M18. Rozdzielone środowiska i plan wycofania** przed każdym wdrożeniem.
**M19. Kopie zapasowe** z przetestowanym odtwarzaniem.

## E6. Testy i kontekst CDD

**M20. Testy są obowiązkowe;** nie zmieniamy oczekiwanych wyników, żeby test przeszedł.
**M21. Naprawiamy przyczyny, nie objawy.**
**M22. Rewalidacja kontekstu AI** po każdym cyklu: kontekst odbudowujemy ze specyfikacji, CHANGELOG, README i testów.

## Język

**M23.** Artefakty i komunikacja po polsku.
