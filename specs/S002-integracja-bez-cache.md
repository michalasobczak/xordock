# S002 — Integracja z macOS bez jawnego cache na dysku

- **Status:** szkic — wymaga sesji specyfikacyjnej z właścicielem
- **Data:** 2026-11-22
- **Powiązane:** S001 (F-01, F-02 w `docs/bezpieczenstwo/2026-11-22-runda-S001.md`)

## 1. TL;DR

Zastąpić montowanie przez WebDAV takim, w którym jawna treść plików nie trafia na wewnętrzny SSD i do sejfu nie da się dostać przez gniazdo TCP. Potwierdzenie: test E2E nie znajduje treści testowej w żadnym pliku poza udziałami (także otwartym i usuniętym), a inny proces bez uprawnień nie może czytać sejfu.

## 2. Abstract

Prototyp S001 montuje sejf przez lokalny serwer WebDAV i `mount_webdav`. Podczas E2E `lsof` pokazał, że `webdavfs_agent` trzyma treść plików w usuniętych plikach w `/private/tmp/.webdavcache.*` (F-01). Token dostępu jest widoczny w tablicy montowań (F-02). Obie luki łagodzą FileVault i model przeciwnika, ale nie spełniają intencji FR-24.

WebDAV wybrano, bo na stanowisku nie było Xcode (FSKit) i nie chcieliśmy rozszerzenia jądra (macFUSE).

## 3. Objective

- **Cele:** integracja bez cache na dysku i bez dostępu przez TCP dla innych procesów; zachowanie wszystkich bramek S001.
- **Poza zakresem:** zmiana formatu udziałów.
- **Kryteria sukcesu:** F-01 i F-02 zamknięte w następnej analizie bezpieczeństwa.

## 4. Artefact Inventory

| Artefakt | Rodzaj | Stan |
| --- | --- | --- |
| `internal/mount` | kod | do zmiany |
| `internal/davfs` | kod | do zmiany albo do usunięcia |
| Xcode + Apple Developer ID (FSKit) | zewnętrzne | [DO USTALENIA] |

## 5. Functional Domain Changes

[DO USTALENIA] Opcje do porównania:

1. **FSKit** (natywne rozszerzenie systemu plików): brak kext, brak TCP, cache w RAM. Wymaga Xcode, podpisu i włączenia rozszerzenia przez użytkownika.
2. **NFS loopback** (`mount_nfs` z lokalnym serwerem NFSv3): klient jądra cache'uje w RAM. Wstępny test: użytkownik bez uprawnień administratora może wywołać `mount_nfs` (próba nie skończyła się błędem uprawnień). Wymaga biblioteki serwera NFS (nowa zależność) i nadal gniazda TCP (F-02 zostaje, chyba że ograniczymy port i uwierzytelnianie).
3. **macFUSE**: odrzucone w S001 (kext, obniżenie zabezpieczeń).

## 6. Critical Path

[DO USTALENIA] po wyborze opcji.

## 7. Control Gates

| # | Bramka | Warunek zaliczenia | Weryfikacja | Blokuje |
| --- | --- | --- | --- | --- |
| G1 | Brak jawnej treści na SSD poza udziałem A | 0 trafień markera w plikach otwartych przez procesy systemowe (`lsof`) i w `/private/tmp`, `/private/var/folders` w trakcie i po pracy | E2E | wydanie |
| G2 | Brak dostępu dla innego procesu bez tokenu/uprawnień | odmowa | E2E | wydanie |
| G3 | Wszystkie bramki CG-01–CG-11 z S001 | zielone | `go test ./...` + E2E | wydanie |

## 8. Expected Outcome

Zamknięte F-01 i F-02; `mount` może przestać wymagać FileVault (nadal zalecany, FR-27).

## 9. Additional Information

- Źródło: analiza bezpieczeństwa rundy S001.
- Otwarte pytanie: czy właściciel może zainstalować Xcode i ma konto Apple Developer (warunek FSKit)?
