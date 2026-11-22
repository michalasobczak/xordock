# S004 — XorDock: bezpieczny katalog macOS z dwoma udziałami i odzyskiwaniem

- **Status:** dostarczona dla CP-01–CP-07 (prototyp CLI + montowanie w Finderze). Otwarte: CP-08 (interfejs SwiftUI) i część CP-09 (fuzz, test nocny 1 GiB, pomiar 1 GiB). Szczegóły w §9 „Realizacja”.
- **Data:** 2026-11-22
- **Powiązane:** zastępuje S001 (silnik przejęty) i S002 (porzucona); S005 (poprawki z analizy bezpieczeństwa tej rundy); S003 (szkic)
- **Źródło:** specyfikacja właściciela wpisana jako `specs/S004.md` (tytuł roboczy SPEC-001). Numer i nazwa pliku według konwencji repozytorium. Tekst właściciela poniżej bez zmian; decyzje wykonawcze dopisane w §9.

> Tekst właściciela:


**Wersja:** 1.0-draft  
**Data:** 2026-11-22  
**Metodyka:** Rondo / Context-Driven Development (CDD) — https://workwithrondo.com/  
**Status:** Gotowe do rozpoczęcia *implementacji prototypu*, nie do przechowywania danych produkcyjnych  
**Odbiorca:** niezależny autor produktu XorDock  
**Repozytorium:** `xordock` (nowe)  
**Docelowy plik:** `specs/SPEC-001-xordock-macos-mvp.md`

## 1. TL;DR

Zbudować na macOS katalog widoczny w Finderze, którego każdy blok zawartości i wrażliwych metadanych jest rozdzielany na dwa udziały XOR: `A` na wewnętrznym SSD Maca oraz `B` na roboczej karcie SD. Dodać migawki wersji udziału A i synchronizację odpowiadających im bloków B na drugą kartę SD, tak aby po zgubieniu roboczej karty dało się odzyskać *ostatni prawidłowy punkt odzyskiwania*, ale nie późniejsze zmiany. Sukces należy potwierdzić automatycznymi testami operacji plikowych, utraty nośnika, symulowanych awarii zasilania i pełnego odzyskiwania.

## 2. Abstract

Projekt zaczyna się bez istniejącej implementacji. Zwykłe szyfrowanie dysku z krótkim kluczem i opcjonalnym tokenem nie spełnia rozważanego celu fizycznego rozdzielenia informacji: sama kopia jednego udziału nie powinna wystarczać do odtworzenia dokumentów.

XorDock realizuje prosty podział danych: dla wiadomości `M` generuje maskę `R`, zapisuje `A = M XOR R` na SSD oraz `B = R` na wyjmowanym nośniku. Rekonstrukcja `M = A XOR B` odbywa się tylko przy dostępie do obu udziałów. Dla wygody system udostępnia zwykły wolumen/katalog użytkownikowi macOS.

W wersji MVP maski pochodzą z systemowego kryptograficznego generatora macOS (CSPRNG). Zapewnia to silny *praktyczny* model bezpieczeństwa, lecz **nie jest dowodem doskonałej tajności ani bezwarunkowej odporności na P = NP**. Wariant ze zweryfikowanym strumieniem niezależnych losowych masek wymaga osobnej specyfikacji i przeglądu kryptograficznego.

Dwa identyczne nośniki nie stanowią RAID 1. Robocza SD zawiera aktywny udział B. Zapasowa SD przechowuje **historyczny udział B** dla ostatniego zatwierdzonego punktu odzyskiwania. Po utracie roboczej SD można odtworzyć tylko tę historyczną wersję, używając zachowanej, zgodnej migawki udziału A. Nie stosujemy migawek całego systemu plików APFS.

Pierwsze wydanie jest lokalnym prototypem na własnym Macu, dystrybuowanym poza Mac App Store. Domyślnie nie wolno wymagać wyłączania SIP/AMFI ani obniżania polityki bezpieczeństwa Apple Silicon.

## 3. Objective

### Goals

- **G-01:** Utworzyć i zamontować w Finderze jeden chroniony wolumen `XorDock`, niezależny od dysku systemowego.
- **G-02:** Zapisywać zawartość plików i wrażliwe metadane wyłącznie jako dwa zgodne udziały, bez trwałych plików jawnych w magazynach XorDock.
- **G-03:** Nie montować wolumenu, jeśli brakuje poprawnego udziału B; nie zastępować brakującego udziału zerami, starymi blokami ani CSPRNG.
- **G-04:** Zapewnić operacje zwykłych plików oraz trwałe, spójne zapisy pomimo dwóch fizycznych nośników.
- **G-05:** Zachowywać historyczne wersje bloków A wyłącznie na potrzeby migawek XorDock.
- **G-06:** Synchronizować zapasową SD raz na żądanie użytkownika (np. co tydzień), kopiując tylko brakujące bloki B punktu odzyskiwania.
- **G-07:** Po zgubieniu roboczej SD odtworzyć spójną wersję folderu z ostatniej zatwierdzonej migawki A i odpowiadającej jej zapasowej SD.
- **G-08:** Zapewnić diagnozę: czy wolumen jest zamontowany, jaki nośnik jest wymagany, kiedy wykonano ostatni sprawdzony backup i jakie zmiany mogą przepaść.
- **G-09:** W sposób weryfikowalny odróżniać poufność teoretycznego XOR z idealnym R od rzeczywistej ochrony implementacji używającej CSPRNG macOS.

### Out of scope — MVP

- Start systemu z chronionego wolumenu; szyfrowanie systemowego SSD, wymiana FileVault.
- iPhone jako backend, SMB/NAS, synchronizacja przez Internet, iCloud i współdzielenie danych.
- Własne rozszerzenie FSKit podpisywane indywidualnym entitlementem.
- Programowe wymazywanie danych z komórek flash jako gwarantowana funkcja bezpieczeństwa.
- Gwarancja ochrony przed przejętym systemem, złośliwą aplikacją czy podsłuchiwaniem pamięci działającego Maca.
- Gwarancja integralności wobec przeciwnika z dowolną mocą obliczeniową; musi być osobno zaprojektowana.
- Pełne bezpieczeństwo informacyjno-teoretyczne produktu przy P = NP: jest wymaganiem badawczym po MVP, a nie deklaracją gotowego produktu.
- Pełna kompatybilność z każdą aplikacją macOS i wszystkimi funkcjami POSIX w pierwszym wydaniu.

### Success criteria

**SC-01:** pliki po zapisaniu i ponownym zamontowaniu są bajtowo identyczne;  
**SC-02:** sam udział A ani sam udział B nie zawiera jawnych testowych dokumentów;  
**SC-03:** odłączenie roboczej SD kończy się deterministycznym błędem operacji i stanem bezpiecznym;  
**SC-04:** po utracie SD roboczej da się odtworzyć ostatni zatwierdzony punkt backupu, nie późniejsze zmiany;  
**SC-05:** dowolna symulowana przerwa w transakcji nie tworzy cicho uszkodzonej „zatwierdzonej” wersji;  
**SC-06:** nie są potrzebne SIP-off, AMFI-off, Reduced Security ani płatne konto Apple do lokalnego prototypu;  
**SC-07:** dokumentacja nie zawiera nieuzasadnionej obietnicy „odporności na P = NP”.

## 4. Artefact Inventory

| Artefakt | Stan | Funkcja |
|---|---|---|
| `README.md` | to create | Aktualny rzeczywisty stan, instalacja, ograniczenia |
| `CHANGELOG.md` | to create | Co faktycznie zbudowano i przetestowano |
| `specs/SPEC-001-xordock-macos-mvp.md` | to create | Ten kontrakt wdrożenia |
| `docs/engineering-manifesto.md` | to create | Zasady jakości, bezpieczeństwa, commitowania i przeglądów |
| `docs/threat-model.md` | to create | Dokładny model zagrożeń i ograniczeń |
| `docs/format-v1.md` | to create | Format udziałów, wersji, katalogu, transakcji i manifestów |
| `docs/recovery.md` | to create | Procedura backupu i odzyskiwania po utracie SD |
| `core/` (Rust) | to create | XOR, obiekty, manifesty, transakcje, migawek i recovery |
| `macos/` (SwiftUI) | to create | Interfejs, wybór SD, mount/unmount, diagnostyka |
| `fuse-adapter/` | to create | Adapter operacji FUSE do `core/` |
| `tests/unit/` | to create | Testy operacji, masek, manifestów |
| `tests/integration/` | to create | Dwa magazyny, spójność transakcji, hot-unplug |
| `tests/recovery/` | to create | Testy odzyskiwania z historycznego punktu |
| `scripts/fault-injection/` | to create | Kontrolowane przerwanie zapisu/commit/backup |
| macFUSE 5.x + backend FSKit | exists (external) | FUSE bez własnego kext, wymagane potwierdzenie na docelowej wersji macOS |
| `SecRandomCopyBytes` | exists (system) | CSPRNG dla prototypu; niebezwarunkowa losowość |
| Testowe SSD/SD oraz nośnik zapasowy | to provide | Fizyczny test pełnego scenariusza |
| Publiczne API surowego hardware TRNG na macOS | not established | Nie przyjmować jako istniejącej zależności |

**Ustalenie platformy:** Apple Silicon, macOS 26 lub nowszy. Podczas startu kodowania odnotować dokładną wersję macOS, Xcode i macFUSE w `README.md` oraz CI. Używać wspieranych API. W fazie produkcyjnej sprawdzić licencję macFUSE — komercyjne bundlowanie lub automatyczne instalowanie dystrybucji binarnej wymaga odrębnej zgody/licencji.

## 5. Functional Domain Changes

### 5.1. Model danych i bezpieczeństwa

**D-01.** Dla każdego nowego logicznego bloku `M` aplikacja żąda niezależnej maski `R` o identycznej liczbie bajtów. `A = M XOR R`, `B = R`. Odzyskanie wymaga obu udziałów tego samego identyfikatora i tej samej wersji.

**D-02.** Bloki są *niezmienne*: ponowny zapis danego fragmentu pliku tworzy nowy identyfikator obiektu, nową maskę i oba nowe udziały. Nie wolno nadpisywać udziału przy zachowaniu wcześniejszej maski ani używać tej samej maski do dwóch wersji.

**D-03.** Początkowy rozmiar bloku: 1 MiB (parametr formatu, z testami dla małych i dużych plików). Końcowy blok może być krótszy; rozmiary pozostają obserwowalną metadaną. Ewolucja formatu wymaga wersjonowania.

**D-04.** Nazwy plików, drzewo katalogów, tekstowe atrybuty i mapowanie pliku na bloki są częścią chronionego katalogu logicznego — dzielimy je na udziały tak samo jak zawartość. Jawne nagłówki nośników mogą zawierać tylko minimum: identyfikator wolumenu, rolę udziału, wersję formatu, numer generacji i niejawne identyfikatory obiektów. Ujawnienie liczby, rozmiarów i rytmu zmian może pozostać możliwe; dokumentujemy to.

**D-05.** Żadne pole w udziale A ani udziale B nie może zawierać jawnego oryginalnego pliku lub jego nazwy. Nie stosować hashy plaintextu jako nazw obiektów. Proste sumy kontrolne *własnego udziału* służą wykrywaniu przypadkowych uszkodzeń, ale nie potwierdzają integralności przy aktywnym ataku.

**D-06.** `RandomSource` ma interfejs `fill(buffer) -> Result`. MVP implementuje adapter systemowego `SecRandomCopyBytes`, sprawdzając kod wyniku; błąd generatora zatrzymuje zapis. Testy używają deterministycznego adaptera jawnie oznaczonego `TEST_ONLY`, który nie może znaleźć się w buildzie wydaniowym. Interfejs do przyszłego źródła fizycznej losowości jest rozszerzeniem, **nie** obietnicą osiągniętej gwarancji.

**D-07.** Zapobieganie wyciekom jawnych danych: silnik nie zapisuje ich do katalogów roboczych, logów, crash-reportów ani warstwy backupu. Nie można zagwarantować, że zewnętrzne aplikacje nie pozostawią plaintextu w temp/cache/Quick Look/swap/Time Machine. Program wyświetla ostrzeżenie i zaleca FileVault.

### 5.2. Interfejs i montowanie

**F-01.** Tworzenie wolumenu: użytkownik wybiera katalog magazynu A na wewnętrznym dysku i przygotowuje roboczą kartę SD jako magazyn B. Inicjalizacja generuje niezwiązany z nazwą wolumenu identyfikator pary. Nie należy zakładać, że zwykła karta SD jest niekopiowalnym tokenem.

**F-02.** Zamontowanie następuje tylko po sprawdzeniu spójnej pary udziałów i wspólnej zatwierdzonej generacji. Gdy jedna strona jest nowsza wskutek awarii, system wybiera ostatnią *wspólnie* zatwierdzoną generację albo odmawia montowania z informacją o naprawie.

**F-03.** Standardowe operacje MVP: listowanie, tworzenie plików/katalogów, odczyt fragmentu, zapis fragmentu, zmianę rozmiaru, rename, delete, `stat`, `flush`, `fsync` i ponowne otwarcie. Obsługa `xattr`, aliasów/symlinków, blokad oraz atomowego zapisu aplikacji wymaga osobnych testów zgodności. Operacje nieobsługiwane mają zwrócić jawny błąd, nie pozorny sukces.

**F-04.** Wirtualny katalog pojawia się w Finderze. Rozłączenie roboczej SD przestawia magazyn w stan `DEGRADED/READ_ONLY_OR_UNAVAILABLE` i odmawia nowych operacji wymagających B. Nie wolno buforować całego udziału B na SSD ani trwale utrzymywać go w cache.

**F-05.** `unmount` kończy nowe otwarcia, synchronizuje aktywne operacje, kończy transakcje, oznacza wolumen jako zamknięty i umożliwia bezpieczne wysunięcie karty. Nie obiecywać usunięcia jawnych danych, które aplikacje już przechowują w swojej pamięci.

### 5.3. Spójny zapis na dwóch nośnikach

**T-01.** Zmiana pliku: zapis nowych bloków A i B pod nowymi ID; utrwalenie obiektów (`fsync`); przygotowanie nowego chronionego katalogu metadanych; utrwalenie obu stron; zapis rekordu `prepared` na obu nośnikach; przejście do wspólnego `committed` i aktualizacja korzenia wolumenu. Format ma być odporny na brak atomowości między dyskami.

**T-02.** Po restarcie skaner wybiera ostatni wspólny poprawny korzeń `committed`, sprawdza kompletność wskazywanych obiektów i odrzuca niedokończone obiekty. `fsync` w widocznym systemie plików nie może potwierdzać trwałości danych, dopóki wymagane udziały nie zostały utrwalone.

**T-03.** Jednoczesne zapisy muszą być serializowane w zakresie modyfikowanych obiektów i korzenia manifestu. Pierwszy prototyp może mieć jednego piszącego na wolumen, jeśli interfejs jasno egzekwuje ograniczenie.

**T-04.** Brak miejsca, brak SD, błąd I/O, odcięcie zasilania i uszkodzenie manifestu: brak cichej utraty ostatniej potwierdzonej wersji; błąd i instrukcja odzyskiwania.

### 5.4. Migawki tylko udziałów XorDock

**S-01.** Migawka jest niezmiennym identyfikatorem zatwierdzonego korzenia katalogu XorDock: `snapshot_id`, `generation`, zakres wymaganych identyfikatorów bloków i znacznik czasu. Powiązany stan katalogu logicznego pozostaje dzielony na A/B.

**S-02.** `createSnapshot()` nie robi snapshotu całego wolumenu APFS i nie kopiuje plaintextu. Przypina historyczne obiekty A i B tak, żeby nadpisanie dokumentu nie usunęło bloków niezbędnych starszej migawce.

**S-03.** Zmiana pliku po migawce tworzy nowe obiekty i nowy katalog logiczny, a stare obiekty pozostają dopóki istnieje wymagająca ich migawka albo backup.

**S-04.** `garbageCollect()` usuwa wyłącznie obiekty nieosiągalne z bieżącego korzenia oraz wszystkich przypiętych migawek/ważnych backupów. Przy braku pewności — zatrzymuje się.

### 5.5. Tygodniowy backup na drugą SD

**B-01.** Workflow: włóż roboczą SD, utwórz migawkę `G`; włóż zapasową SD; XorDock kopiuje *brakujące* bloki udziału B dla `G` oraz odpowiadające im wersjonowane metadane udziału B. Wymagane bloki udziału A pozostają przypięte lokalnie.

**B-02.** Backup ma osobny identyfikator nośnika i identyfikator punktu odzyskiwania, ale nie jest osobnym kluczem kryptograficznym. Kopiuje udział B w tej samej wersji. Kopia B na drugą SD nie jest ponownym szyfrowaniem tym samym OTP; to skopiowanie tego samego udziału.

**B-03.** Nowy backup powstaje w obszarze staging. System potwierdza go jako `READY` dopiero po sprawdzeniu obecności i poprawności wszystkich wymaganych obiektów. **W razie błędu poprzedni, sprawdzony punkt odzyskiwania musi pozostać kompletny.**

**B-04.** Jeśli na zapasowej SD brak miejsca na bezpieczny staging, synchronizacja odmawia działania zamiast usuwać jedyny dobry backup. Usunięcie starej wersji wymaga osobnej decyzji retencyjnej użytkownika i nowego, zakończonego sukcesem punktu.

**B-05.** Po zakończeniu aplikacja pokazuje datę i ID punktu `READY`, zakres gwarantowanego odzyskania oraz ostrzeżenie, że późniejsze zmiany **nie** znajdują się na zapasowej karcie.

**B-06.** Backup A na osobnym fizycznym nośniku jest przyszłą funkcją. Lokalna migawka A + zapasowa SD chronią przed utratą **roboczej SD**, lecz nie przed awarią lub utratą SSD Maca.

### 5.6. Odzyskiwanie

**R-01.** Użytkownik wkłada zapasową SD po zgubieniu roboczej. XorDock pokazuje zgodne migawki udziału A i punkty B oznaczone jako `READY`.

**R-02.** `restoreSnapshot(G)` wymaga dokładnie zgodnego zestawu obiektów A i B tej samej generacji `G`; nie miesza `A_G` z `B_(G+N)` i nie „uzupełnia” ich zgadywanymi danymi.

**R-03.** Odzyskiwanie przeprowadza pełny odczyt i kontrolę kompletności do nowego zestawu udziałów lub do wskazanego, jawnego folderu eksportowego. Nie może modyfikować ani usuwać jedynej sprawdzonej kopii zapasowej.

**R-04.** Po odzyskaniu użytkownik może utworzyć nową roboczą SD z kopii udziału B, a następnie kontynuować pracę w nowej generacji. Zgubioną kartę traktować jako potencjalnie skopiowaną — nie obiecywać jej zdalnego unieważnienia.

### 5.7. Integralność, poufność i zagrożenia

- **I-01:** Główne zapewnienie z teorii informacji dotyczy samej *poufności* jednego idealnie losowego udziału, nie dostępności, integralności, bezpieczeństwa hosta ani dowodu pochodzenia danych.
- **I-02:** Dla MVP sprawdzać integralność przypadkową: sumy kontrolne każdej części liczone na tej samej części, zgodność ID, rozmiaru, wersji i korzeni. Sam checksum nie chroni przed celową podmianą udziału. Mechanizm autentyczności przed atakującym modyfikującym nośnik wymaga osobnej decyzji kryptograficznej, która nie może podważyć własności pojedynczego udziału.
- **I-03:** Nie umieszczać na pojedynczym udziale jawnych znaczników, hashy czy MAC-ów liczonych z **plaintextu**, które mogłyby ujawnić informację o pliku.
- **I-04:** Program ma dokumentować ryzyko wycieku przez pamięć RAM, swap, Quick Look, indeksowanie Spotlight, zapis tymczasowy aplikacji, crash dump, kopię Time Machine, backup iCloud i instalowane rozszerzenia. Funkcje ograniczające takie wycieki są dodatkowym etapem, nie założeniem domyślnym.
- **I-05:** Wyciągnięcie SD roboczej, gdy wszystkie dane są już jawne w aplikacji, nie usuwa ich z pamięci Maca. Nie gwarantować odporności na malware ani użytkownika administracyjnego.
- **I-06:** Nazwy i pojemność plików w aktywnym folderze mogą pojawić się poza magazynem udziałów wskutek działania systemu i aplikacji — obowiązkowy threat model musi to uwzględniać.

## 6. Critical Path

1. **CP-01 / Start projektu:** stworzyć repozytorium, README, manifest, niniejszą spec, testy i bazowy threat model. Nie projektować UI przed ustaleniem modelu danych.
2. **CP-02 / Minimalny silnik:** testowy CLI `init`, `put`, `get` dla dwóch katalogów na osobnych lokalizacjach testowych; maska CSPRNG, XOR, jawne błędy, testy bajtowe.
3. **CP-03 / Immutable object store:** obiekty A/B + wersjonowany podzielony katalog logiczny, nowe maski przy zmianach, brak nadpisania wersji.
4. **CP-04 / Transakcje:** utrwalenie obu udziałów, wspólny korzeń, recovery po przerwanym zapisie i zbieranie śmieci oparte na referencjach.
5. **CP-05 / Migawki:** `snapshot create/list/pin`, generacje, niezmienne historyczne bloki A/B, bez APFS-volume snapshot.
6. **CP-06 / Recovery:** backup na zapasową SD, `READY`, test utraty roboczej SD, przywrócenie ostatniego punktu i stworzenie nowej roboczej SD.
7. **CP-07 / FUSE:** wyłącznie po pozytywnym spike sprawdzającym macFUSE 5.5 `backend=fskit` na docelowej wersji macOS; adapter FUSE do stabilnego Core bez logiki kryptograficznej w adapterze.
8. **CP-08 / UX:** SwiftUI, wybór nośników, montowanie, backup/restore, historia punktów i czytelne komunikaty.
9. **CP-09 / Walidacja:** testy awarii, fuzz, retencja, pomiar szybkości, analiza bezpieczeństwa; dopiero potem demonstracja na niesensytywnych plikach.

**Koniec ścieżki krytycznej:** użytkownik może korzystać z plików testowych w Finderze, wykonać poprawny punkt odzyskiwania, zgubić roboczą SD i odzyskać stan katalogu z tej migawki bez ręcznego XOR.

## 7. Control Gates

Każda bramka podaje jednoznaczną weryfikację. **Błąd dowolnej bramki blokuje oznaczenie etapu jako ukończonego**; wszystkie testy powinny działać automatycznie w CI, z wyjątkiem jawnie oznaczonych testów sprzętowych.

| Gate | Warunek PASS | Rodzaj dowodu |
|---|---|---|
| CG-01 | `get(put(X)) == X` dla 0 B, 1 B, 4 KiB, 1 MiB, 5 MiB, 1 GiB (test opcjonalny nocny) i plików binarnych | Unit/integration |
| CG-02 | Każda edycja tworzy inne ID maski; żadna maska nie jest używana w dwóch wersjach bloku | Unit/property |
| CG-03 | Brak poprawnej SD uniemożliwia montowanie; błędna SD jest odrzucana | Integration/E2E |
| CG-04 | Po `fsync` oba udziały i manifest wytrzymują czysty restart i test odtworzenia | Integration |
| CG-05 | Przerwanie po każdym kroku zapisu/commit kończy się ostatnim wspólnym zatwierdzonym stanem albo jednoznacznym błędem | Fault injection |
| CG-06 | `createSnapshot()` zapisuje historyczny korzeń, bez snapshotu całego APFS i bez kopiowania całego plaintextu | Integration |
| CG-07 | `snapshot G1` i późniejszy zapis `G2` pozwalają nadal odczytać dokładne `G1` przy poprawnych udziałach | Integration |
| CG-08 | Zapasowa SD `B_G1` + migawka `A_G1` odtwarzają dokładne bajty z `G1` | Recovery E2E |
| CG-09 | Zapasowa SD `B_G1` nie jest błędnie używana do odtworzenia nowych bloków z `A_G2` | Negative recovery |
| CG-10 | Przerwanie backupu pozostawia poprzedni `READY`; nie oznacza przerwanego punktu jako `READY` | Fault injection |
| CG-11 | Brak miejsca na backup nie kasuje ostatniej działającej kopii i zwraca błąd | Negative integration |
| CG-12 | Brak stale widocznego plaintextu w magazynach A/B, logach silnika i nazwach obiektów, w ramach zdefiniowanego testu | Security inspection |
| CG-13 | Nie ma wywołań wymagających wyłączenia SIP/AMFI; mount działa na docelowym Macu przez backend FSKit macFUSE | Manual hardware gate |
| CG-14 | `README.md` i UI określają CSPRNG jako gwarancję **praktyczną**, a nie informacyjno-teoretyczną | Documentation review |
| CG-15 | Po każdej rundzie jest zapisane zagrożenie, przegląd regresji bezpieczeństwa i test lub uzasadnienie braku testu | Security review |
| CG-16 | Zależność macFUSE ma zweryfikowane warunki licencji dla rodzaju dystrybucji; komercyjne bundlowanie bez zgody jest zabronione | License review |

**Zakazy release (negative gates):** nie wolno zareklamować MVP jako „P=NP-proof”; nie wolno automatycznie usuwać ostatniego backupu; nie wolno dać aplikacji prawa zapisu do wszystkich nośników bez świadomego wyboru użytkownika; nie wolno włączyć kext/reduced security tylko dla poprawienia wydajności; nie wolno używać prawdziwych tajnych dokumentów przed przejściem bramek awarii i recovery.

## 8. Expected Outcome

**Direct effect:** Na Macu powstaje katalog XorDock działający po podłączeniu roboczej karty SD. Pliki są zapisane jako rozdzielone udziały na SSD i SD. Użytkownik może tworzyć migawki chronionego katalogu, synchronizować zapasową SD i przywrócić ostatni punkt po utracie roboczej karty.

**Measurable effect:** mierzyć przyrost miejsca dla jednego punktu odzyskiwania, liczbę zmienionych bloków do synchronizacji, odczyt/zapis małych i dużych plików, czas montowania i odtwarzania 1 GiB. Nie deklarować progów wydajności przed pomiarem. Dla wybranych testów odtworzenie musi dawać identyczny SHA-256 plaintextu *w narzędziu testowym*, nie przechowywany jako publiczny znacznik na jednym udziale.

**Side effects:** wymagana podwójna objętość aktywnych udziałów, dodatkowa historia A, opóźnienie SD, ryzyko awarii nośnika, brak dostępności bez karty, brak odtwarzania zmian po dacie backupu, ryzyko jawnych kopii poza XorDock.

**Rollback:** Program ma udostępniać świadomy eksport jawnych plików przy obecności obu udziałów. Nie usuwa ich automatycznie ani nie twierdzi, że eksport jest nadal zabezpieczony. Po eksporcie użytkownik może bezpiecznie odmontować XorDock. Deinstalacja aplikacji bez eksportu nie gwarantuje zachowania dostępu do danych.

## 9. Additional Information

### ADR — decyzje techniczne

- **ADR-001:** Silnik Rust, niezależny od UI/FSKit/FUSE; adapter FUSE wyłącznie tłumaczy operacje plikowe na API silnika.
- **ADR-002:** SwiftUI dla zarządzania wolumenem i nośnikami. Pierwszy milestone może być wyłącznie CLI.
- **ADR-003:** macFUSE >=5.5.0 z `backend=fskit` jako pierwszy kandydat. Wykonać test montowania i testy zgodności przed związaniem architektury. Nie wymagamy własnego entitlementu `com.apple.developer.fskit.fsmodule`, ponieważ nie tworzymy własnego rozszerzenia FSKit.
- **ADR-004:** CSPRNG Apple przez `SecRandomCopyBytes` i jawne sprawdzenie statusu; nie zakładać dostępu do surowego TRNG w Secure Enclave.
- **ADR-005:** Copy-on-write realizowany we własnym obiektowym store i manifestach, *nie* przez snapshoty wolumenu APFS. APFS `clonefile()` wyłącznie jako późniejsza optymalizacja.
- **ADR-006:** W MVP tylko dwa fizyczne udziały A/B. Zapasowa SD jest kopią B **konkretnej wersji**, a nie trzecim udziałem Shamira.
- **ADR-007:** Prototyp jest niezależny od Mac App Store. Przy komercyjnej integracji macFUSE sprawdzić warunki licencji i w razie potrzeby uzyskać zgodę/licencję lub zmienić warstwę montowania.

### Założenia i konsekwencje bezpieczeństwa

- Zdobycie jednego *idealnie losowego* udziału jest informacyjno-teoretycznie niewystarczające do odtworzenia danych. Nie oznacza to, że wynik `SecRandomCopyBytes` ma doskonałą tajność OTP.
- Karta SD jest kopiowalna; „fizyczna obecność karty” nie znaczy, że żaden przeciwnik nie skopiował udziału B wcześniej.
- Brak fizycznej separacji (np. dwa katalogi na tym samym SSD) nie chroni przed skopiowaniem całego SSD.
- Zapasowa B w sejfie nie zabezpiecza przed awarią SSD Maca; do tego potrzebny jest osobny backup migawki A, przechowywany niezależnie.
- Do doskonałej tajności 100 GB dowolnych danych potrzebny jest odpowiednio długi pełnoentropijny materiał dla każdej wersji ochrony; testy statystyczne pseudolosowego strumienia nie dowodzą tej własności.
- Wariant pod P = NP wymaga osobnej specyfikacji dla walidacji źródła rzeczywistej losowości, ochrony historii wersji, integralności i kompleksowego audytu wycieków.

### Rejected alternatives

1. Całodyskowy XorDock — odrzucony dla pierwszej wersji ze względu na boot, zgodność i trudne odzyskiwanie.
2. Migawki całego APFS — odrzucone, bo chcemy wersjonować tylko chroniony udział i nie ingerować w cały wolumen.
3. Zwykły plik klucza AES na SD — nie realizuje fizycznego podziału informacji.
4. Klucz Shamir 2 z 3 dla samych krótkich sekretów — nie zapewnia doskonałej tajności samych plików przy P = NP.
5. iPhone/NAS jako aktywny backend — po MVP; wymaga dodatkowego projektu transportu i modelu podsłuchu.
6. Własny natywny FSKit od pierwszego dnia — niepotrzebny do prototypu i zwiększa obciążenia podpisywaniem/uprawnieniami.

### Otwarte kwestie blokujące produkt produkcyjny, nie prototyp

- Dokładna obsługa spójności Finder/Quick Look, `xattr`, `mmap`, Office autosave, SQLite-on-FUSE i aplikacji tworzących pliki tymczasowe.
- Integracja bezpośrednio z FSKit zamiast macFUSE, jeśli licencja, wydajność lub model dystrybucji tego wymaga.
- Poziom autentyczności i integralności wobec złośliwego modyfikowania udziału.
- Reguły retencji, backup A, szczegóły odtwarzania po utracie obydwu nośników.
- Zewnętrzne źródło fizycznej losowości oraz walidacja pełnoentropijnej ilości danych.
- Niezależny audyt bezpieczeństwa i jakości źródła losowości.

### Kontekst i referencje

- Rondo: https://workwithrondo.com/ — dziewięć sekcji, artifacts `specs/`, `README`, `CHANGELOG`, testy oraz przegląd bezpieczeństwa po zmianach.
- macFUSE: https://macfuse.github.io/ i https://github.com/macfuse/macfuse/wiki/FUSE-Backends — backend FSKit, `backend=fskit`, ograniczenia API.
- macFUSE License: https://github.com/macfuse/macfuse/blob/release/macfuse/LICENSE.txt — ograniczenia komercyjnego bundlowania binariów.
- Apple SecRandomCopyBytes: https://developer.apple.com/documentation/security/secrandomcopybytes(_:_:_:)
- Apple Platform Security: https://support.apple.com/guide/security/random-number-generation-seca0c73a75b/web

### Proces Rondo / CDD

Dla każdej kolejnej funkcji utworzyć nowy plik `specs/SPEC-00N-*.md` z dziewięcioma sekcjami. Po implementacji aktualizować `CHANGELOG.md` wyłącznie tym, co faktycznie ukończono, oraz `README.md` zgodnie ze stanem kodu. Testy i bramki bezpieczeństwa mają być dowodem dostarczenia zmiany. Nie zamykać specyfikacji, jeśli krytyczna luka ma status „do naprawienia później”.


### Realizacja (2026-11-22, dopisane przez wykonawcę)

#### Decyzje właściciela i odstępstwa

| ID | Decyzja | Uzasadnienie |
|---|---|---|
| A-01 | **Rdzeń w Go, nie w Rust** (odstępstwo od ADR-001). Decyzja właściciela 2026-11-22 po pytaniu wykonawcy. | Silnik z S001 (udziały, commit A→B, odzyskiwanie po awarii, GC) był przetestowany; przepisanie dałoby drugą implementację formatu (MANIFEST M2). Rust z Homebrew był też uszkodzony na stanowisku. Go jest bezpieczny pamięciowo jak Rust; adapter FUSE nie zawiera kryptografii (ADR-001 zachowany co do podziału odpowiedzialności). |
| A-02 | Projekt **nie** startuje od zera (§2 mówi inaczej): S004 przejmuje silnik S001 i rozszerza go. Inwentarz §4 mapujemy na istniejące artefakty: `core/` → `internal/split`, `internal/vault`; `fuse-adapter/` → `internal/fusefs` (logika) + `internal/fusemount` (most cgo do libfuse3); `tests/*` → `*_test.go` przy pakietach; `scripts/fault-injection/` → haki `Options.Hook` w testach; `docs/engineering-manifesto.md` → istniejący `MANIFEST.md`. | Zgodność z konwencją repozytorium (testy Go przy pakietach, jeden manifest). |
| A-03 | Maski z `SecRandomCopyBytes` przez cgo, kod wyniku sprawdzany (D-06 1:1). Deterministyczne źródło TEST_ONLY leży tylko w pliku `_test.go`, więc nie może trafić do binarki. | D-06, ADR-004. |
| A-04 | Migawki zapisane w indeksie (dzielonym XOR), nie w jawnym pliku. Każda zmiana listy migawek to zwykły commit A→B. | D-04 (metadane chronione), T-01 (ten sam protokół spójności). |
| A-05 | T-01 „prepared/committed”: realizuje to istniejący protokół: obiekty + fsync, bariera `F_FULLFSYNC` na obu nośnikach, superblok A (stan przygotowany), superblok B (zatwierdzony). Przy otwarciu: dokończ albo wycofaj; inne stany to jawny błąd (F-02, T-02). | Bez zmiany formatu; pokryte `TestCrashAtEveryStage`. |
| A-06 | `chmod`/`chown` przyjmowane bez skutku (odstępstwo od F-03 „jawny błąd”). Uprawnienia nie są przechowywane: pliki 0600, katalogi 0700, właściciel montujący. | Backend FSKit macFUSE ustawia tryb przy każdym `mkdir`/`create`; błąd z `chmod` blokował tworzenie plików (E2E 2026-11-22). |
| A-07 | `rename` z flagą zamiany miejscami (`RENAME_SWAP`) zwraca `ENOTSUP`; `RENAME_EXCL` obsługiwany. `xattr`, symlinki, twarde linki: nieobsługiwane (ENOSYS/ENOTSUP). | F-03: brak pozornego sukcesu. |
| A-08 | Backup wymaga **jednocześnie** karty roboczej i zapasowej (druga przez czytnik USB). B-01 opisuje zamianę kart w jednym gnieździe, ale udziałów B nie wolno buforować na SSD (F-04, G-02), a bufor w RAM dla dużej migawki jest niepraktyczny. | Bezpieczeństwo ponad wygodę; do rewizji w CP-08. |
| A-09 | Odzyskiwanie (`recover restore`) tworzy **nowy sejf z nowymi maskami** na nowej karcie, zamiast kopiować udział B na nową kartę roboczą (R-04). | Zgubiona karta może być skopiowana (R-04). Przy kopii B jej maski pasowałyby do odtworzonych udziałów A. Stary udział A i karta zapasowa zostają nietknięte (R-03); program zaleca usunięcie starego A po sprawdzeniu. |
| A-10 | Retencja: `backup prune` zostawia tylko najnowszy punkt READY i zdejmuje przypięcia starszych; migawkę przypiętą przez punkt można usunąć dopiero po prune. Nic nie dzieje się automatycznie (B-04). | Jawna decyzja użytkownika. |
| A-11 | Migawki i backup tylko przy odmontowanym sejfie (CLI bierze tę samą blokadę `flock` co montowanie). | T-03: jeden piszący na wolumen. |
| A-12 | macFUSE 5.5.0 zainstalowany przez właściciela do użytku prywatnego (prototyp). CG-16: licencja BSD z klauzulą 4: bundlowanie binarki macFUSE z oprogramowaniem komercyjnym (także automatyczna instalacja) wymaga pisemnej zgody autora. xordock **nie** bundluje ani nie instaluje macFUSE. | ADR-007. Przed dystrybucją komercyjną: zgoda albo zmiana warstwy montowania. |

#### Środowisko (CP-01)

macOS 27.0 (26A428), Apple Silicon, Go 1.27.4, Xcode 27.2 (cgo/clang), macFUSE 5.5.0 (libfuse3 3.18.3, backend FSKit, rozszerzenie włączone w Ustawieniach systemowych). Karta robocza: SD HFS+ w czytniku MacBooka Pro.

#### Bramki i dowody

| Gate | Dowód | Wynik |
|---|---|---|
| CG-01 | `TestRoundTripAfterReopen` (0 B…3 MiB+5), `TestRandomWritesMatchModel` (300 losowych zapisów/obcięć przez adapter), `TestLargeFileSpills`; E2E: 5 MB, 30 MiB losowych danych, `cmp` po montowaniu | ✅ (1 GiB nocny: otwarte, CP-09) |
| CG-02 | `TestEveryWriteGetsFreshMask`, `TestTestOnlySourceDeterministic`, `TestMaskErrorStopsSplit` | ✅ |
| CG-03 | `TestMissingCardBlocksOpen`, `TestWrongCardRejected`, `TestRecoverAfterLosingWorkingCard` (karta zapasowa odrzucona jako robocza) | ✅ |
| CG-04 | `TestRoundTripAfterReopen`, `TestRandomWritesMatchModel` (ponowne otwarcie + fsck); E2E: fsck po odmontowaniu | ✅ (zob. R4-03: fsync przez FSKit) |
| CG-05 | `TestCrashAtEveryStage` (13 scenariuszy), `TestTornTempFilesCleaned`, `TestInterruptedInitFinished` | ✅ |
| CG-06 | `TestSnapshotKeepsOldVersion` (migawka dodaje tylko obiekt indeksu) | ✅ |
| CG-07 | `TestSnapshotKeepsOldVersion` | ✅ |
| CG-08 | `TestRecoverAfterLosingWorkingCard`; E2E: backup na drugi nośnik, `recover verify/export/restore` bez karty roboczej, `cmp` 30 MiB | ✅ |
| CG-09 | `TestSpareNotUsedForNewerObjects`, `TestRecoverAfterLosingWorkingCard` | ✅ |
| CG-10 | `TestInterruptedBackupKeepsPreviousPoint` (3 etapy) | ✅ |
| CG-11 | `TestBackupNoSpace` | ✅ |
| CG-12 | `TestSharesHoldNoPlaintext`, `TestSpareHoldsNoPlaintext`, `TestLogsHoldNoSecrets`; E2E: brak markera i nazw w A, B, karcie zapasowej, nowym sejfie, `/private/tmp`, `/private/var/folders` | ✅ |
| CG-13 | E2E 2026-11-22: `mount` z `backend=fskit`, bez kext, SIP/AMFI włączone, bez płatnego konta Apple | ✅ (ręcznie) |
| CG-14 | `TestKindIsHonest`; komunikaty CLI, README | ✅ |
| CG-15 | `docs/bezpieczenstwo/2026-11-22-runda-S004.md` | ✅ |
| CG-16 | A-12 | ✅ (prototyp prywatny) |

#### Pomiary (E2E, karta SD HFS+, SSD z FileVault)

Zapis przez Findera/FSKit: 30 MiB w 2,9 s (~10 MiB/s, jak silnik w S001). Mały plik (utworzenie + zapis + zamknięcie): ~220 ms. Wykrycie wyjęcia karty i odmontowanie: ≤ 1,25 s. Migawka: 1 obiekt (indeks). Backup 30 MiB (33 obiekty) na obraz dysku: < 2 s.
