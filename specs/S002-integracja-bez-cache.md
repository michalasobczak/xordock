# S002 — Katalog w Finderze przez FSKit

- **Status:** porzucona (2026-11-22). Własne rozszerzenie FSKit wymaga płatnego Apple Developer Program; właściciel wybrał S004 (macFUSE z backendem FSKit, bez własnego uprawnienia). Projekt `macos/` usunięty przed commitem. G1 i G2 sprawdzono w S004 (E2E, analiza rundy S004).
- **Data:** 2026-11-22
- **Powiązane:** S001 (F-01, F-02 w `docs/bezpieczenstwo/2026-11-22-runda-S001.md`), S003

## 1. TL;DR

Sejf jest widoczny w Finderze jako zwykły wolumen dzięki natywnemu rozszerzeniu systemu plików macOS (FSKit), bez lokalnego serwera sieciowego i bez plików cache z jawną treścią na SSD. Potwierdzenie: wszystkie bramki S001 przechodzą przez nowy wolumen, a test E2E nie znajduje treści testowej poza RAM i udziałami.

## 2. Abstract

Prototyp S001 montował sejf przez lokalny WebDAV. Analiza bezpieczeństwa wykazała, że agent WebDAV macOS zapisuje treść plików na SSD (F-01), a token dostępu widać w tablicy montowań (F-02).

Historia decyzji (2026-11-22): rozważono NFS loopback (cache klienta w RAM). Prototyp serwera NFSv3 dotarł do próby montowania i został przerwany. Właściciel odrzucił NFS i WebDAV: integracja ma być natywna. Kod WebDAV i NFS usunięto, więc `xordock mount` nie istnieje. Wcześniej FSKit blokował brak Xcode; właściciel zainstalował Xcode 27.2.

Zostawiono stałe numery plików w indeksie sejfu (`Entry.Ino`) i operacje po numerze (`ByIno`, `SetFileIno`, `Create`, `Remove`, `RenameReplacing`), bo FSKit identyfikuje elementy stałymi identyfikatorami.

## 3. Objective

- **Cele:** rozszerzenie FSKit (moduł systemu plików) montujące sejf, aplikacja-kontener, która je dostarcza, obsługa karty jak w S001 (SC-02, FR-18).
- **Poza zakresem:** zmiana formatu udziałów; akcja „Chroń kartą SD” w Finderze (S003).
- **Kryteria sukcesu:** bramki CG-01–CG-11 z S001 przez wolumen FSKit; G1 i G2 poniżej; F-01 i F-02 zamknięte w następnej analizie bezpieczeństwa.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| Xcode 27.2, macOS SDK 27.2 z FSKit | zewnętrzne | istnieje |
| Certyfikat Apple Development (Personal Team `F5AKYMU3G6`) + pośredni Apple WWDR G3 | zewnętrzne | istnieje (2026-11-22), podpis działa |
| Uprawnienie `com.apple.developer.fskit.fsmodule` | zewnętrzne | **blokada**: Personal Team go nie obsługuje; wymagany płatny Apple Developer Program |
| `macos/Xordock.xcodeproj`, `macos/Xordock/XordockApp.swift` (aplikacja-kontener), `macos/XordockFS/` (moduł „hello” tylko do odczytu, Info.plist z `FSSupportsPathURLs`, entitlements) | kod | istnieje, kompiluje się; nie da się podpisać bez profilu |
| `internal/vault` (silnik sejfu w Go) | kod | istnieje |
| Most Go → Swift (`go build -buildmode=c-archive`) | kod | do utworzenia (rekomendacja, zob. §9) |
| Projekt Xcode: aplikacja + rozszerzenie FSKit (Swift) | kod | do utworzenia |
| Włączenie rozszerzenia przez użytkownika (Ustawienia systemowe) | organizacja | do wykonania przez właściciela |

## 5. Functional Domain Changes

[DO USTALENIA] na sesji. Do rozstrzygnięcia:

- jak użytkownik montuje sejf (aplikacja w pasku menu, `mount -F`, automatycznie po włożeniu karty);
- gdzie w FSKit buforować zapisy i kiedy zatwierdzać (zamknięcie pliku, `fsync`, bezczynność), żeby zachować FR-10 i FR-11;
- zachowanie przy wyjęciu karty w trakcie pracy.

### Wynik kroku 1 (2026-11-22)

`xcodebuild -allowProvisioningUpdates`: *„Personal development teams … do not support the FSKit Module capability.”* Bez płatnego konta Apple nie wyda profilu z uprawnieniem FSKit, a macOS nie uruchomi rozszerzenia bez niego (obejście wymaga wyłączenia SIP/AMFI, co odrzucamy: obniża bezpieczeństwo całego Maca, E4).

## 6. Critical Path

1. Tożsamość do podpisu i potwierdzenie, że rozszerzenie FSKit z uprawnieniem się ładuje (minimalny moduł „hello”).
2. Most do silnika Go i operacje tylko do odczytu.
3. Zapis, zmiana nazwy, usuwanie, zatwierdzanie.
4. Karta: nadzór i odmontowanie.
5. Testy E2E bramek S001, G1, G2; analiza bezpieczeństwa.

## 7. Control Gates

| # | Bramka | Warunek zaliczenia | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | Brak jawnej treści na SSD poza RAM | 0 trafień markera w plikach otwartych przez procesy systemowe (`lsof`) oraz w `/private/tmp` i `/private/var/folders` w trakcie i po pracy | E2E | wydanie |
| G2 | Brak dostępu przez sieć lub gniazdo | rozszerzenie nie otwiera żadnego gniazda nasłuchującego | `lsof -i` | wydanie |
| G3 | Bramki CG-01–CG-11 z S001 | zielone | `go test ./...` + E2E | wydanie |

## 8. Expected Outcome

Sejf montowany natywnie, F-01 i F-02 zamknięte.

## 9. Additional Information

- **Odrzucone:** WebDAV (F-01, F-02), NFS loopback (decyzja właściciela), macFUSE (rozszerzenie jądra, obniżenie zabezpieczeń).
- **Rekomendacja mostu:** silnik sejfu zostaje w Go i jest linkowany do rozszerzenia jako biblioteka C. Przepisanie silnika na Swift dałoby dwie implementacje formatu (M2: bez duplikacji). Osobny proces pomocniczy przez XPC to dodatkowa granica do zabezpieczenia.
- **Otwarte pytanie:** czy właściciel dołącza do Apple Developer Program (99 USD/rok)? Bez tego S002 jest zablokowana.
