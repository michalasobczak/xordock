# Backup and recovery

> Backup writes one recovery point in two places: B shares to the spare card and A shares to iCloud Drive (S008). Each of the three media (SSD, working card, spare card) can be lost one at a time. You recover the state from the **last READY recovery point**; later changes are lost.

| Lost | You recover from |
| --- | --- |
| working card | A on the SSD + spare card |
| SSD | share A copy in iCloud Drive + spare or working card |
| spare card | nothing is lost; make a new backup |

## Backup (e.g. weekly)

The vault must be unmounted. Both cards connected at the same time (the second through a USB reader).

```bash
xordock snapshot create --name sejf
xordock backup --name sejf --spare /Volumes/ZAPAS
xordock status --name sejf      # date of the last READY recovery point and the number of changes since then
```

- The A shares of the same snapshot go to `iCloud Drive/xordock/<name>` (`~/Library/Mobile Documents/com~apple~CloudDocs/xordock/<name>`). `--no-icloud` skips this copy. The iCloud account + any B card is a complete set: enable 2FA and Advanced Data Protection, and keep the cards separately.
- Only the snapshot's missing shares are copied; nothing is deleted or overwritten. If there is not enough space, the backup refuses and the previous point remains.
- The snapshot with a point is pinned: its A shares stay on the SSD.
- Retention (a deliberate decision): `xordock backup prune --name sejf --spare /Volumes/ZAPAS` keeps only the newest verified point, on the card and in iCloud. After that old snapshots can be deleted: `xordock snapshot rm --id …`.

## After losing the working card

```bash
xordock recover --name sejf --spare /Volumes/ZAPAS list
xordock recover --name sejf --spare /Volumes/ZAPAS verify
xordock recover --name sejf --spare /Volumes/ZAPAS restore --card /Volumes/NOWA
# or a plaintext export (the files are NOT protected):
xordock recover --name sejf --spare /Volumes/ZAPAS export --to /Volumes/DYSK/odzysk
```

- `restore` creates a new vault `<name>-recovered` with **new masks** on the new card. The spare card and the old share A are left untouched.
- Treat the lost card as copied. After checking the new vault, delete **all copies of the old share A**, because each of them together with the lost card makes a complete set:
  - `~/Library/Application Support/xordock/<name>` on the SSD;
  - `iCloud Drive/xordock/<name>`, then empty "Recently Deleted" in iCloud (iCloud.com → Drive), because the files stay there for another 30 days;
  - copies in Time Machine (`tmutil delete` or deletion in Time Machine).
- The same rule applies after losing the **spare card**: restore the vault into a new one (`restore`, new masks) and delete all copies of the old A.
- After recovery, take a new snapshot and back up the new vault to the spare card (a different directory on the card, because the new vault has a different name).

Desktop and Documents are sometimes synced with iCloud. Direct a plaintext export outside them, otherwise the files will end up in the cloud without protection.

## After losing the SSD (or on a new Mac)

```bash
xordock recover --from-icloud --name sejf --spare /Volumes/ZAPAS verify
xordock recover --from-icloud --name sejf --spare /Volumes/ZAPAS restore --card /Volumes/NOWA --new-name sejf
```

- Share A comes from the copy in iCloud Drive. If the spare card was lost too, pass the working card in `--spare`.
- In the app: the "Share A copies in iCloud without SSD" section → "Recovery…".
- Recovery changes nothing in the A copy. `restore` creates a new vault with new masks. After checking it, back up the new vault.
