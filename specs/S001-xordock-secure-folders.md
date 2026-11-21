# S001 — xordock Secure Folders for macOS

- **Status:** dostarczona (prototyp) — decyzje w §9.8, wyniki w §9.9. **Integrację WebDAV (`xordock mount`, `internal/davfs`, `internal/mount`) usunięto 2026-11-22** na decyzję właściciela; montowanie w Finderze przejmuje S002 (FSKit). §9.8–9.9 opisują stan z commitu `98e5fdc`.
- **Data:** 2026-11-22
- **Projekt:** xordock
- **Platforma:** macOS, Apple Silicon
- **Typ:** nowa aplikacja / wirtualny system plików
- **Zakres:** pierwszy używalny prototyp katalogu opartego na dwóch udziałach danych
- **Powiązane:** S002 (integracja bez cache na dysku)

## 1. TL;DR

Chcę stworzyć aplikację na macOS, która udostępnia użytkownikowi zwykły katalog w Finderze, ale przechowuje jego zawartość jako dwa niezależne udziały: jeden na wewnętrznym SSD Maca, drugi na wyjmowanej karcie SD.

Bez karty SD odtworzenie zawartości katalogu nie może być możliwe z samego udziału na Macu. Prototyp ma wykazać poprawność działania i obsługi awarii, a uzyskanie gwarancji poufności niezależnej od P = NP wymaga osobnej, udokumentowanej walidacji źródła losowości i całego modelu bezpieczeństwa.

## 2. Abstract

Współczesne rozwiązania ochrony plików na macOS wykorzystują przede wszystkim szyfrowanie oparte na krótkich kluczach kryptograficznych. Fizyczny token może kontrolować dostęp do takiego klucza, ale nie gwarantuje matematycznej poufności danych w przypadku znalezienia praktycznej metody złamania algorytmu szyfrującego.

xordock ma realizować inny model: zawartość każdego chronionego pliku zostaje rozdzielona na dwa udziały za pomocą operacji XOR i świeżej maski losowej. Pierwszy udział przechowywany jest na Macu, a drugi na karcie SD. Oba są niezbędne do odtworzenia danych.

Użytkownik nie powinien wykonywać ręcznie operacji dzielenia ani składania plików. Powinien korzystać ze zwykłego katalogu widocznego w Finderze.

Projekt obejmuje wybrane katalogi, a nie cały dysk systemowy. Nie wymaga podłączania karty SD do uruchamiania macOS.

Rozważono istniejące projekty SafeFS i Lecithine, jednak nie przyjęto założenia, że ich implementacje spełniają wymagania bezpieczeństwa produkcyjnego.

Największymi ryzykami są jakość losowości, spójność zapisów na dwóch nośnikach, pozostawianie jawnych kopii plików przez aplikacje oraz niezawodność kart SD.

## 3. Objective

### Goals

**G-01 — Wirtualny katalog.** System ma udostępniać chroniony katalog w macOS jako zwykłą lokalizację dostępną z Findera i standardowych aplikacji.

**G-02 — Dwa fizyczne udziały.** Każdy blok chronionych danych ma być reprezentowany przez dwa udziały:

- A — na lokalnym dysku Maca;
- B — na wyjmowanej karcie SD.

Obowiązuje zależność:

```
A = M XOR R
B = R
M = A XOR B
```

gdzie `M` to oryginalne dane, a `R` to maska losowa.

**G-03 — Wymagana karta SD.** Bez poprawnej karty SD chroniony katalog nie może zostać zamontowany ani odtworzony z samego udziału A.

**G-04 — Normalna praca użytkownika.** Użytkownik ma móc tworzyć, odczytywać, modyfikować, przenosić i usuwać pliki bez ręcznego zarządzania udziałami.

**G-05 — Odporność na awarie.** Przerwanie zapisu nie może powodować cichego odtworzenia nieprawidłowych danych ani niespójnego katalogu bez komunikatu o błędzie.

**G-06 — Jawnie określone bezpieczeństwo.** Produkt musi odróżniać:

- prototyp korzystający z kryptograficznie bezpiecznego generatora pseudolosowego (CSPRNG);
- wariant docelowy, wymagający pełnowymiarowej, niezależnej losowości do uzyskania doskonałej tajności udziałów.

Nie wolno reklamować wariantu CSPRNG jako matematycznie odpornego na P = NP.

### Out of scope

Pierwsza specyfikacja nie obejmuje:

- szyfrowania całego dysku systemowego;
- ochrony przed malware na odblokowanym Macu;
- ochrony przed atakującym posiadającym jednocześnie oba udziały;
- synchronizacji przez iCloud;
- współdzielenia katalogów przez Internet;
- dedykowanych operatorów xordock;
- trzech udziałów i schematów 2-z-3;
- gotowego produktu z certyfikowaną odpornością na P = NP.

### Success criteria

- **SC-01.** Użytkownik tworzy chroniony katalog, montuje go, zapisuje i odczytuje pliki, a ich zawartość po ponownym montowaniu jest identyczna.
- **SC-02.** Usunięcie karty SD uniemożliwia nowe odczyty udziałów wymagających jej obecności.
- **SC-03.** Po ponownym uruchomieniu Maca katalog pozostaje niedostępny do czasu podłączenia właściwej karty i przeprowadzenia autoryzacji.
- **SC-04.** Każda modyfikacja bloku otrzymuje nową maskę, bez ponownego wykorzystania poprzedniej.
- **SC-05.** Awaria podczas zapisu kończy się odzyskaniem ostatniej zatwierdzonej wersji albo jednoznacznym błędem, nigdy cichą korupcją.
- **SC-06.** Zakończone zostają testy funkcjonalne, testy awarii i przegląd bezpieczeństwa dla zakresu prototypu.

## 4. Artefact Inventory

| Artefakt | Stan | Odpowiedzialność |
|---|---|---|
| Repozytorium xordock | istnieje | Kod i historia zmian |
| `README.md` | istnieje | Aktualny stan projektu |
| `specs/S001-xordock-secure-folders.md` | istnieje | Niniejsza specyfikacja |
| `CHANGELOG.md` | istnieje | Faktycznie dostarczone funkcje |
| `MANIFEST.md` | istnieje (do uzupełnienia o decyzje S001) | Zasady implementacji i bezpieczeństwa |
| `docs/threat-model.md` | do utworzenia | Model przeciwnika i granice ochrony |
| Aplikacja macOS | do utworzenia | Montowanie i obsługa katalogu |
| Warstwa wirtualnego systemu plików | do utworzenia | Operacje plikowe i rekonstrukcja |
| Silnik podziału danych | do utworzenia | Tworzenie i składanie udziałów |
| Warstwa losowości | do utworzenia | Dostarczanie masek i walidacja źródła |
| Magazyn udziału A | do utworzenia | Dane na wewnętrznym SSD |
| Magazyn udziału B | do utworzenia | Dane na karcie SD |
| Mechanizm spójności transakcji | do utworzenia | Bezpieczne zatwierdzanie zmian |
| Narzędzie diagnostyczne i odzyskiwania | do utworzenia | Kontrola spójności bez ujawniania treści |
| Testy automatyczne | do utworzenia | Dowody działania i regresji |
| Dokumentacja użytkownika | do utworzenia | Bezpieczne korzystanie z karty |
| Ocena FSKit / macFUSE | [DO USTALENIA] | Wybór wspieranej integracji z macOS |
| Specyfikacja źródła rzeczywistej losowości | [DO USTALENIA] | Osobny warunek wariantu informacyjno-teoretycznego |

Stan repozytorium na 2026-11-22: tylko artefakty CDD, bez kodu.

## 5. Functional Domain Changes

### 5.1. Stan docelowy

Użytkownik widzi jeden chroniony katalog. Fizycznie pliki mają dwa udziały przechowywane na różnych nośnikach. Aplikacje otrzymują jawne dane tylko podczas autoryzowanego korzystania z zamontowanego katalogu.

### 5.2. Tworzenie katalogu

- **FR-01.** Użytkownik wskazuje lokalizację udziału A na Macu.
- **FR-02.** Użytkownik wskazuje kartę SD przeznaczoną na udział B.
- **FR-03.** System sprawdza tożsamość nośnika, możliwość zapisu, ilość miejsca i zgodność z wymaganym formatem.
- **FR-04.** Aplikacja inicjalizuje nowy chroniony katalog i zapisuje powiązanie jego udziałów.
- **FR-05.** Użytkownik ustanawia dodatkową autoryzację dostępu, np. hasło, o ile nie osłabia ona mechanizmu podziału sekretu.
- **FR-06.** Katalog nie może zostać utworzony częściowo bez możliwości jednoznacznego anulowania lub dokończenia inicjalizacji.

### 5.3. Zapisywanie danych

- **FR-07.** Zapis każdego nowego bloku wymaga świeżej maski.
- **FR-08.** Każda maska może zostać zastosowana do utworzenia udziałów tylko jednej wersji danego bloku.
- **FR-09.** Udział A i B muszą zostać zapisane w zgodnych wersjach.
- **FR-10.** Zapis zostaje uznany za trwały dopiero po zakończeniu wymaganych operacji utrwalenia i zatwierdzenia obu udziałów.
- **FR-11.** Ograniczenie miejsca lub brak dostępu do karty SD musi zatrzymać zapis przed ogłoszeniem powodzenia.

### 5.4. Odczytywanie danych

- **FR-12.** System odczytuje odpowiednie fragmenty z obu udziałów.
- **FR-13.** Oryginalny blok jest odtwarzany w pamięci operacyjnej operacją XOR.
- **FR-14.** Niezgodność identyfikatorów, wersji lub struktury udziałów skutkuje błędem, a nie zwróceniem przypadkowych danych.
- **FR-15.** Program nie może celowo utrwalać jawnych danych w katalogu technicznym ani dzienniku diagnostycznym.

### 5.5. Obsługa karty SD

- **FR-16.** System rozpoznaje właściwy nośnik i właściwy zestaw udziałów, nie polegając wyłącznie na nazwie wolumenu.
- **FR-17.** Podłączenie innej karty nie może spowodować połączenia udziałów pochodzących z różnych katalogów.
- **FR-18.** Utrata połączenia z kartą podczas zapisu musi wprowadzać katalog w stan błędu i uniemożliwiać kolejne zatwierdzenia zmian.
- **FR-19.** Bezpieczne odmontowanie wymaga zakończenia aktywnych operacji i utrwalenia danych.
- **FR-20.** Po odmontowaniu nowe odczyty i zapisy nie są dostępne. Dane wcześniej odczytane do pamięci aplikacji nie mogą być uznawane za automatycznie usunięte.

### 5.6. Zmiany i historia plików

- **FR-21.** Nadpisanie pliku nie może wykorzystywać ponownie maski z poprzedniej wersji.
- **FR-22.** Operacje zmiany rozmiaru, usuwania, przenoszenia i zmiany nazwy muszą zachowywać zgodność udziałów.
- **FR-23.** System musi jawnie określać zachowanie wobec wersjonowania, kopii zapasowych, dzienników i migawek.
- **FR-24.** Niedozwolone jest tworzenie ukrytej pełnej kopii jawnego katalogu na którymkolwiek nośniku.

### 5.7. Model bezpieczeństwa

**Przeciwnik objęty docelową gwarancją:**

- zdobywa pełną kopię udziału A;
- może mieć nieograniczoną moc obliczeniową, w tym algorytmy wynikające z praktycznego P = NP;
- zna pełny kod i format danych;
- może analizować wcześniejsze kopie udziału A;
- nie uzyskuje udziału B ani jawnych kopii danych;
- nie kontroluje komputera podczas pracy z zamontowanym katalogiem.

**Gwarancja matematyczna dotyczy poufności zawartości**, przy założeniu niezależnych, jednolicie losowych masek o odpowiedniej długości i prawidłowej realizacji protokołu.

W szczególności pojedynczy udział nie powinien ujawniać treści danych. Nie oznacza to automatycznie ukrywania rozmiarów, czasu aktywności, informacji z systemu operacyjnego ani odporności na modyfikowanie udziałów przez atakującego.

**Wariant CSPRNG** nie spełnia warunku bezwarunkowej poufności. Ma być jednoznacznie oznaczony jako praktyczny prototyp, bez deklaracji odporności na P = NP.

### 5.8. Bezpieczeństwo danych poza katalogiem

- **FR-25.** Aplikacja ma dokumentować ograniczenia związane z plikami tymczasowymi, pamięcią wymiany, podglądami Findera, historią ostatnich dokumentów i zachowaniem aplikacji biurowych.
- **FR-26.** System nie może obiecywać, że wszystkie programy macOS usuwają swoje jawne kopie po zamknięciu dokumentu.
- **FR-27.** Zalecane jest niezależne używanie FileVault jako dodatkowej ochrony pozostałych danych komputera.

## 6. Critical Path

Celem jest uzyskanie jednego działającego katalogu, którego zawartość można odczytać tylko przy dostępności dwóch zgodnych udziałów.

1. **CP-01 — Model danych.** Zdefiniować format i zasady wersjonowania udziałów. Potwierdzić testami, że świeże maski tworzą udziały poprawnie oraz że nie są ponownie używane.
2. **CP-02 — Operacje plikowe.** Zaimplementować podstawowe tworzenie, odczytywanie, zapisywanie i usuwanie plików na dwóch lokalizacjach testowych.
3. **CP-03 — Integracja macOS.** Wybrać i zweryfikować wspierany mechanizm wirtualnego systemu plików. Udostępnić katalog w Finderze.
4. **CP-04 — Karta SD.** Zastąpić jedną lokalizację testową rzeczywistą kartą SD. Zweryfikować jej podłączanie, identyfikację, odmontowywanie i ponowne montowanie.
5. **CP-05 — Spójność zapisów.** Zaimplementować odzyskiwanie po niepełnej operacji i sprawdzić zachowanie podczas symulowanych awarii.
6. **CP-06 — Testy bezpieczeństwa.** Sprawdzić izolację udziałów, brak ponownego użycia masek, zakres jawnych kopii i odporność na nieprawidłowe dane wejściowe.
7. **CP-07 — Prototyp demonstracyjny.** Użytkownik zapisuje plik do katalogu w Finderze, odmontowuje go, odłącza kartę i nie może ponownie otworzyć katalogu. Po podłączeniu właściwej karty może odczytać plik bez zmian.

**Warunek ukończenia:** spełnione wszystkie bramki MVP, bez niezamkniętych błędów powodujących utratę danych w przewidzianym modelu testowym.

## 7. Control Gates

Wszystkie bramki są obowiązkowe dla zadeklarowanego zakresu. Bramka niespełniona blokuje oznaczenie odpowiedniego wydania jako gotowego.

| ID | Warunek przejścia | Weryfikacja |
|---|---|---|
| CG-01 | Odczyt po zapisie daje identyczne bajty | Testy jednostkowe i E2E dla plików o różnych rozmiarach |
| CG-02 | Sama kopia udziału A nie zawiera jawnej treści testowych plików | Inspekcja nośnika i testy scenariuszy |
| CG-03 | Sama kopia udziału B nie zawiera jawnej treści testowych plików | Inspekcja nośnika i testy scenariuszy |
| CG-04 | Każda modyfikacja otrzymuje nową maskę | Testy instrumentowane z jednoznacznymi identyfikatorami masek |
| CG-05 | Brak karty blokuje dostęp do katalogu | Test E2E |
| CG-06 | Niewłaściwa karta jest odrzucana | Test E2E |
| CG-07 | Awaria zapisu nie powoduje cichej korupcji | Testy awaryjne z przerwaniem w każdym etapie zatwierdzania |
| CG-08 | Równoległe operacje nie powodują pomieszania udziałów | Testy współbieżności |
| CG-09 | Logi nie zawierają treści plików ani masek | Automatyczna analiza logów testowych |
| CG-10 | CSPRNG nie jest przedstawiany jako gwarancja odporności na P = NP | Weryfikacja dokumentacji i interfejsu |
| CG-11 | Analiza bezpieczeństwa bieżącej i poprzednich specyfikacji nie wykrywa niezamkniętych regresji krytycznych | Przegląd bezpieczeństwa po każdej rundzie |

### Dodatkowe bramki dla przyszłego wariantu P = NP

- **IT-01 — Walidacja entropii.** Udokumentowane źródło maski musi dostarczać wystarczającą ilość niezależnej entropii dla całego chronionego bloku. Samo przejście testów statystycznych losowości nie stanowi dowodu.
- **IT-02 — Dowód poufności.** Niezależna analiza kryptograficzna musi potwierdzić, że jeden udział nie ujawnia treści w zdefiniowanym modelu przeciwnika, także przy uwzględnieniu aktualizacji i historii zapisów.
- **IT-03 — Weryfikacja granic systemu.** Muszą zostać zbadane kanały ujawnienia treści poza magazynami udziałów, w szczególności pamięć wymiany, cache aplikacji, indeksowanie i kopie zapasowe.
- **IT-04 — Uczciwe deklaracje.** Dopóki IT-01–IT-03 nie zostaną spełnione, żadna wersja programu nie może być opisywana jako zapewniająca pełną odporność na P = NP.

## 8. Expected Outcome

### Efekt bezpośredni

Powstaje aplikacja na macOS umożliwiająca utworzenie i używanie katalogu, którego dane fizycznie wymagają dwóch nośników. System operacyjny nadal działa bez karty SD. Po jej odłączeniu chroniony katalog staje się niedostępny dla nowych operacji.

### Efekt mierzalny

W prototypie muszą zostać zmierzone:

- przepustowość odczytu i zapisu;
- opóźnienia operacji na małych plikach;
- czas montowania i odmontowywania;
- narzut pojemnościowy na obu nośnikach;
- skuteczność wykrywania przerwanych zapisów.

Wyniki mają zostać zapisane w dokumentacji testów. Progi wydajności dla wydania produkcyjnego zostaną ustalone na podstawie pomiarów prototypu, a nie zgadywanych wartości.

**Pomiary prototypu (2026-11-22, MacBook Pro Apple Silicon, macOS 27.0, A: wewnętrzny SSD z FileVault, B: karta SD HFS+ w wbudowanym czytniku):**

| Pomiar | Wynik | Jak powtórzyć |
|---|---|---|
| Zapis 20 MiB, warstwa sejfu | 1,9 s (10,4 MiB/s) | `XORDOCK_PERF_A=… XORDOCK_PERF_B=/Volumes/… go test -run TestPerf -v ./internal/vault` |
| Odczyt 20 MiB, warstwa sejfu | 33 ms (~600 MiB/s) | jw. |
| Mały plik, warstwa sejfu (1 commit) | ~120 ms | jw. |
| `cp` 20 MB do zamontowanego katalogu | 1,85 s | E2E, §9.9 |
| Mały plik przez montowanie (2 commity) | ~215 ms | E2E, §9.9 |
| Montowanie / odmontowanie | 1,0 s / 0,02 s | E2E, §9.9 |
| Narzut pojemnościowy | 2 × dane + 57 B na obiekt (blok ≤ 1 MiB) + indeks | format w `internal/vault/format.go` |
| Wykrywanie przerwanych zapisów | 100% scenariuszy testowych (5 etapów × kolejne wystąpienia) | `TestCrashAtEveryStage` |

Pierwsza wersja z blokiem 64 KiB dawała 1,4 MiB/s (4 × fsync na blok). Blok 1 MiB i utrwalanie katalogów raz na commit dały ~7× przyspieszenie bez zmiany gwarancji. Wąskim gardłem jest liczba wywołań fsync na karcie SD; małe pliki są ograniczone czasem commitu (2 × F_FULLFSYNC superbloków).

### Skutki uboczne

- Utrata jednego udziału może oznaczać nieodwracalną utratę danych.
- Karty SD mogą być wolniejsze i mniej trwałe niż wewnętrzne SSD.
- W czasie pracy jawne dane są dostępne aplikacjom w pamięci RAM.
- Wydajność zależy od wolniejszego nośnika oraz liczby operacji synchronizacji.
- Zwykła karta SD może zostać skopiowana; nie jest automatycznie bezpiecznym tokenem sprzętowym.

### Wycofanie

Wycofanie programu nie może wymagać pozostawienia jawnej kopii danych na wewnętrznym SSD. Przed wycofaniem użytkownik musi mieć możliwość świadomego eksportu plików do wskazanej lokalizacji po podłączeniu obu udziałów. Eksport musi wyraźnie informować, że pliki docelowe nie będą już chronione mechanizmem podziału sekretu.

## 9. Additional Information

### 9.1. Odrzucone alternatywy

- **Cały dysk systemowy:** odrzucony dla MVP ze względu na złożoność uruchamiania macOS, sterowników i odzyskiwania po awarii.
- **Karta z samym kluczem AES:** nie realizuje celu informacyjno-teoretycznej ochrony udziałów przy P = NP.
- **Tradycyjny RAID 1:** przechowuje pełne kopie danych, więc utrata pojedynczego nośnika ujawnia całość.
- **OTP do komunikacji sieciowej:** poza zakresem tej specyfikacji.
- **Przeniesienie gotowego SafeFS bez audytu:** odrzucone, ponieważ wymaga osobnego sprawdzenia generatora losowości, współczesnej zgodności z macOS i bezpieczeństwa zapisów.

### 9.2. Dług techniczny i ryzyka

- **R-01.** Wybranie CSPRNG dla prototypu oznacza brak ścisłej gwarancji P = NP.
- **R-02.** Zwykłe aplikacje mogą pozostawiać jawne kopie dokumentów poza wirtualnym katalogiem.
- **R-03.** Karta SD może zostać niespodziewanie odłączona.
- **R-04.** Podział XOR sam nie gwarantuje integralności ani autentyczności danych.
- **R-05.** Backup jednego udziału nie wystarcza do odzyskania danych.
- **R-06.** Wyciek rozmiarów, czasów dostępu oraz pewnych metadanych nie jest automatycznie wyeliminowany.

### 9.3. Zależności zewnętrzne

- Mechanizm wirtualnego systemu plików obsługiwany przez wybrane wydania macOS.
- Nośniki SD zapewniające wymagane operacje trwałego zapisu.
- Dostępne narzędzia testowania błędów I/O i awarii zasilania.
- Niezależny przegląd projektu kryptograficznego.
- Osobna ocena źródła entropii dla wariantu informacyjno-teoretycznego.

### 9.4. Otwarte pytania — wymagają decyzji

- **Q-01.** Czy pierwsza wersja ma obsługiwać wyłącznie najnowsze wydanie macOS, czy również starsze wspierane wersje?
- **Q-02.** Czy chroniony katalog ma być dostępny przez FSKit, macFUSE czy inny wspierany interfejs? Decyzję należy podjąć po porównaniu stabilności i uprawnień.
- **Q-03.** Czy prototyp ma używać CSPRNG wyłącznie do demonstracji, a tryb informacyjno-teoretyczny powstanie jako osobna specyfikacja?
- **Q-04.** Jak obsługiwać spójne kopie zapasowe obu udziałów, żeby przypadkowa awaria SD nie oznaczała trwałej utraty danych?
- **Q-05.** Jakie wymagania integralności obowiązują wobec atakującego, który może modyfikować jeden udział? Jest to osobny problem od poufności i wymaga jawnej decyzji kryptograficznej.
- **Q-06.** Czy pierwsza wersja ma obsługiwać edycję dowolnych dokumentów w aplikacjach tworzących automatyczne kopie i pliki tymczasowe, czy tylko uzgodnioną listę programów?

### 9.5. Proces

Projekt jest prowadzony zgodnie z Rondo / CDD (zob. `CLAUDE.md`): każda zmiana ma własną specyfikację w `specs/`, `README.md` opisuje faktyczny stan, `CHANGELOG.md` dokumentuje zrealizowane zmiany, wymagania prowadzą do testów i bramek, a po każdej rundzie wykonywana jest analiza bezpieczeństwa bieżącej i wcześniejszych funkcjonalności. Wykryta luka wymaga udokumentowania, testu odtwarzającego problem i poprawki przed zamknięciem rundy.

### 9.6. Materiały źródłowe

- Rondo / Context-Driven Development: https://workwithrondo.com/
- SafeFS: https://github.com/safecloud-project/SafeFS
- Lecithine: https://sourceforge.net/projects/leci/
- Dokumentacja Apple dotycząca FSKit i mechanizmów bezpieczeństwa macOS.
- Literatura dotycząca one-time pad, doskonałej tajności i podziału sekretów.

### 9.7. Historia nazwy

Robocza nazwa w pierwszym szkicu: „OpenTrust Secure Folders” / „OpenTrust Storage”. Decyzja właściciela (2026-11-22): projekt i produkt nazywają się **xordock**.

### 9.8. Decyzje (2026-11-22)

Właściciel polecił przyjąć rekomendacje bez dalszych pytań.

- **Q-01:** tylko macOS 27 (Apple Silicon), czyli wersja, na której pracujemy.
- **Q-02:** prototyp montuje katalog przez lokalny serwer WebDAV (127.0.0.1) i wbudowany w macOS `mount_webdav`. Powody: brak Xcode na stanowisku (FSKit wymaga rozszerzenia aplikacji podpisanego przez Apple Developer), macFUSE wymaga rozszerzenia jądra i obniżenia zabezpieczeń na Apple Silicon. WebDAV działa bez uprawnień administratora. **Koszt:** agent WebDAV macOS buforuje treść plików na SSD (znalezisko F-01). Docelowa integracja to S002.
- **Q-03:** prototyp używa CSPRNG (`crypto/rand`). Tryb informacyjno-teoretyczny to osobna specyfikacja (IT-01–IT-04 bez zmian).
- **Q-04:** w prototypie bez automatycznych kopii. Zalecenie w `docs/uzytkowanie.md`: kopia katalogu sejfu z karty na drugą kartę przechowywaną osobno, przy odmontowanym sejfie. Pełne rozwiązanie w osobnej specyfikacji.
- **Q-05:** każdy udział ma własną sumę SHA-256 oraz nagłówek wiążący go z sejfem, stroną i obiektem. To wykrywa uszkodzenia przypadkowe i pomieszanie udziałów. **Nie** wykrywa celowej podmiany (XOR jest plastyczny, R-04). Skrót udziału A zapisany na B (i odwrotnie) został odrzucony, bo pozwoliłby nieograniczonemu przeciwnikowi z jednym udziałem weryfikować hipotezy o treści (łamie G-06). Integralność wobec przeciwnika to osobna specyfikacja.
- **Q-06:** brak listy aplikacji; ograniczenia opisane w `docs/uzytkowanie.md` (FR-25, FR-26).
- **FR-05 (hasło):** odłożone do osobnej specyfikacji. W prototypie autoryzacją (SC-03) jest właściwa karta i zalogowana sesja użytkownika macOS.
- **Język implementacji:** Go, jedna zależność: `golang.org/x/net/webdav` (oficjalny moduł Go).
- **Blokada:** sejf otwiera naraz tylko jeden proces (`flock` na `lock` w udziale A).
- **FileVault:** `xordock mount` odmawia działania bez FileVault (flaga `--allow-no-filevault` świadomie to obchodzi), bo F-01.

### 9.9. Realizacja i dowody

| Bramka | Dowód | Wynik |
|---|---|---|
| CG-01 | `TestRoundTripAfterReopen`, `TestWebDAVLifecycle`, `TestRandomWritesMatchModel`; E2E: `cp`/`cmp` 20 MB i 3 MB przez montowanie, po ponownym montowaniu | ✅ |
| CG-02 | `TestSharesHoldNoPlaintext`; E2E: `grep` markera i nazwy pliku w udziale A na SSD | ✅ |
| CG-03 | jw., udział B na karcie | ✅ |
| CG-04 | `TestEveryWriteGetsFreshMask` (instrumentowane źródło masek, 16 masek, 0 powtórzeń), `TestFreshMaskEachSplit` | ✅ |
| CG-05 | `TestMissingCardBlocksOpen`; E2E: odmontowanie karty → katalog odmontowany w ≤ 1 s, `mount` bez karty odrzucony | ✅ |
| CG-06 | `TestWrongCardRejected` | ✅ |
| CG-07 | `TestCrashAtEveryStage` (13 scenariuszy: każde wystąpienie każdego etapu przy zapisie 4 bloków i indeksu), `TestInterruptedInitFinished`, `TestTornTempFilesCleaned`, `TestCorruptShareIsError`, `TestCorruptSuperIsError` | ✅ w modelu testowym; brak testu z fizycznym odcięciem zasilania |
| CG-08 | `TestConcurrentWrites`, `TestConcurrentPUT` (z `-race`), `TestSecondOpenLocked` | ✅ |
| CG-09 | `TestLogsHoldNoSecrets`, `TestStripPaths` | ✅ |
| CG-10 | `TestKindIsHonest`; komunikat „Maski: CSPRNG (prototyp)… BEZ gwarancji odporności na P = NP” przy `init` i `mount` | ✅ |
| CG-11 | `docs/bezpieczenstwo/2026-11-22-runda-S001.md` | ✅ bez regresji krytycznych; F-01 (wysokie) otwarte → S002 |

CP-07 (demonstracja) wykonano przez `mount_webdav`, tę samą ścieżkę, której używa Finder: zapis, odmontowanie, brak karty → brak dostępu, karta z powrotem → plik bez zmian. Nie klikano w Finderze (praca bez obecności właściciela).

---

**Warunek zamknięcia specyfikacji:** zatwierdzony zakres MVP, ustalona integracja z macOS, przyjęty model bezpieczeństwa prototypu oraz zaakceptowane bramki CG-01–CG-11.

**Status bezpieczeństwa P = NP:** cel badawczy, niepotwierdzony dla pierwszego wydania. Wymaga osobnego procesu walidacji IT-01–IT-04.
