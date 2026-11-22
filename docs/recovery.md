# Backup i odzyskiwanie

> Karta zapasowa chroni przed utratą **karty roboczej**, nie przed awarią SSD Maca (B-06). Odzyskujesz stan z **ostatniego punktu READY**; zmiany późniejsze przepadają.

## Backup (np. co tydzień)

Sejf musi być odmontowany. Obie karty podłączone jednocześnie (druga przez czytnik USB).

```bash
xordock snapshot create --name sejf
xordock backup --name sejf --spare /Volumes/ZAPAS
xordock status --name sejf      # data ostatniego punktu READY i liczba zmian od niego
```

- Kopiowane są tylko brakujące udziały B migawki; nic nie jest usuwane ani nadpisywane. Przy braku miejsca backup odmawia i poprzedni punkt zostaje.
- Migawka z punktem jest przypięta: jej udziały A zostają na SSD.
- Retencja (świadoma decyzja): `xordock backup prune --name sejf --spare /Volumes/ZAPAS` zostawia tylko najnowszy, sprawdzony punkt. Potem stare migawki można usunąć: `xordock snapshot rm --id …`.

## Po utracie karty roboczej

```bash
xordock recover --name sejf --spare /Volumes/ZAPAS list
xordock recover --name sejf --spare /Volumes/ZAPAS verify
xordock recover --name sejf --spare /Volumes/ZAPAS restore --card /Volumes/NOWA
# albo jawny eksport (pliki NIE są chronione):
xordock recover --name sejf --spare /Volumes/ZAPAS export --to ~/Desktop/odzysk
```

- `restore` tworzy nowy sejf `<nazwa>-odzyskany` z **nowymi maskami** na nowej karcie. Karta zapasowa i stary udział A zostają nietknięte.
- Zgubioną kartę traktuj jako skopiowaną. Po sprawdzeniu nowego sejfu usuń stary udział A (`~/Library/Application Support/xordock/<nazwa>`): bez niego zgubiona karta nic nie odsłania.
- Po odzyskaniu zrób nową migawkę i backup nowego sejfu na kartę zapasową (inny katalog na karcie, bo nowy sejf ma inną nazwę).
