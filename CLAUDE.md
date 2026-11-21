# xordock — instrukcje dla asystenta

Pracujemy zgodnie z metodyką opisaną na https://workwithrondo.com (Rondo + Context-Driven-Development).

- **Najpierw pytania.** Przed implementacją dopytuję (metodą sokratejską), aż cel, zakres i kryteria akceptacji są jasne. Nie zgaduję wymagań. Decyzje właściciela zapisuję w specyfikacji.
- **Specyfikacja przed kodem.** Każda zmiana ma specyfikację w `specs/` o stałej strukturze 9 sekcji (szablon: `specs/SZABLON.md`, numeracja `SNNN-nazwa.md`).
- **Po dostarczeniu zmiany:** testy potwierdzające bramki kontrolne, wpis w `CHANGELOG.md` (według dat, najnowsze na górze), aktualizacja stanu w `README.md`, status specyfikacji „dostarczona”.
- **Analiza bezpieczeństwa po każdej rundzie.** Gdy specyfikacja jest zaimplementowana i testy przechodzą, sam, bez proszenia, przeprowadzam analizę bezpieczeństwa tej i wszystkich poprzednich specyfikacji: regresje i luki. Każde znalezisko poprawiamy jak zwykłą zmianę (specyfikacja, test odtwarzający problem, wpis w CHANGELOG). Runda kończy się dopiero po analizie.
- **Zasady inżynierskie** są w `MANIFEST.md` (na bazie Engineering essentials E1–E6 z workwithrondo.com). Obowiązują przy każdej specyfikacji. Odstępstwo trzeba nazwać i uzasadnić w sekcji „Additional Information”.
- **Odtwarzalność.** Inny nowoczesny asystent AI powinien móc odbudować projekt z `specs/`, `CHANGELOG.md`, `README.md` i testów.
- Artefakty i komunikacja po polsku.

## Projekt

- Budowanie: `go build -o bin/xordock ./cmd/xordock`. Testy: `go test -race ./...` (muszą przechodzić przed każdym commitem).
- Karta SD do testów E2E: `/Volumes/xordock`. Na karcie zapisujemy tylko w `xordock/<nazwa-sejfu>`; testowe sejfy usuwamy po testach.
- Stan bezpieczeństwa i otwarte znaleziska: `docs/bezpieczenstwo/` (najnowszy raport).
