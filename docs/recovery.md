# Backup i odzyskiwanie

> Backup zapisuje jeden punkt odzyskiwania w dwóch miejscach: udziały B na kartę zapasową i udziały A do iCloud Drive (S008). Każdy z trzech nośników (SSD, karta robocza, karta zapasowa) można stracić pojedynczo. Odzyskujesz stan z **ostatniego punktu READY**; zmiany późniejsze przepadają.

| Utracone | Odzyskujesz z |
| --- | --- |
| karta robocza | A na SSD + karta zapasowa |
| SSD | kopia A w iCloud + karta zapasowa albo robocza |
| karta zapasowa | nic nie ginie; zrób nowy backup |

## Backup (np. co tydzień)

Sejf musi być odmontowany. Obie karty podłączone jednocześnie (druga przez czytnik USB).

```bash
xordock snapshot create --name sejf
xordock backup --name sejf --spare /Volumes/ZAPAS
xordock status --name sejf      # data ostatniego punktu READY i liczba zmian od niego
```

- Udziały A tej samej migawki trafiają do `iCloud Drive/xordock/<nazwa>` (`~/Library/Mobile Documents/com~apple~CloudDocs/xordock/<nazwa>`). `--no-icloud` pomija tę kopię. Konto iCloud + dowolna karta B to komplet: włącz 2FA i Advanced Data Protection, a karty trzymaj osobno.
- Kopiowane są tylko brakujące udziały migawki; nic nie jest usuwane ani nadpisywane. Przy braku miejsca backup odmawia i poprzedni punkt zostaje.
- Migawka z punktem jest przypięta: jej udziały A zostają na SSD.
- Retencja (świadoma decyzja): `xordock backup prune --name sejf --spare /Volumes/ZAPAS` zostawia tylko najnowszy, sprawdzony punkt, na karcie i w iCloud. Potem stare migawki można usunąć: `xordock snapshot rm --id …`.

## Po utracie karty roboczej

```bash
xordock recover --name sejf --spare /Volumes/ZAPAS list
xordock recover --name sejf --spare /Volumes/ZAPAS verify
xordock recover --name sejf --spare /Volumes/ZAPAS restore --card /Volumes/NOWA
# albo jawny eksport (pliki NIE są chronione):
xordock recover --name sejf --spare /Volumes/ZAPAS export --to /Volumes/DYSK/odzysk
```

- `restore` tworzy nowy sejf `<nazwa>-odzyskany` z **nowymi maskami** na nowej karcie. Karta zapasowa i stary udział A zostają nietknięte.
- Zgubioną kartę traktuj jako skopiowaną. Po sprawdzeniu nowego sejfu usuń **wszystkie kopie starego udziału A**, bo każda z nich razem ze zgubioną kartą daje komplet:
  - `~/Library/Application Support/xordock/<nazwa>` na SSD;
  - `iCloud Drive/xordock/<nazwa>`, potem opróżnij „Ostatnio usunięte” w iCloud (iCloud.com → Drive), bo pliki leżą tam jeszcze 30 dni;
  - kopie w Time Machine (`tmutil delete` albo usunięcie w Time Machine).
- Ta sama zasada obowiązuje po zgubieniu **karty zapasowej**: odtwórz sejf do nowego (`restore`, nowe maski) i usuń wszystkie kopie starego A.
- Po odzyskaniu zrób nową migawkę i backup nowego sejfu na kartę zapasową (inny katalog na karcie, bo nowy sejf ma inną nazwę).

Biurko i Dokumenty bywają synchronizowane z iCloud. Jawny eksport kieruj poza nie, bo inaczej pliki trafią do chmury bez ochrony.

## Po utracie SSD (albo nowym Macu)

```bash
xordock recover --from-icloud --name sejf --spare /Volumes/ZAPAS verify
xordock recover --from-icloud --name sejf --spare /Volumes/ZAPAS restore --card /Volumes/NOWA --new-name sejf
```

- Udział A pochodzi z kopii w iCloud Drive. Gdy karta zapasowa też zginęła, w `--spare` podaj kartę roboczą.
- W aplikacji: sekcja „Kopie A w iCloud bez SSD” → „Odzyskiwanie…”.
- Odzyskiwanie niczego nie zmienia w kopii A. `restore` tworzy nowy sejf z nowymi maskami. Po sprawdzeniu zrób backup nowego sejfu.
