# Format v1

Code: `internal/vault/format.go`, `vault.go`, `snapshot.go`, `backup.go`. Format version: `1` (the `format` field in the header). Fields added in S004 (`role`, `media`, snapshots in the index) are optional, so vaults from S001 open without migration.

## Roots

| Root | Where | Contents |
|---|---|---|
| A | SSD, by default `~/Library/Application Support/xordock/<name>` | `xordock.json`, `super`, `obj/`, `lock` |
| B (working card) | `<card>/xordock/<name>` | `xordock.json`, `super`, `obj/` |
| spare card | `<spare card>/xordock/<name>` | `xordock.json` (role `backup`), `obj/` (copies of B shares), `points/` |

## Header `xordock.json` (plaintext)

`format`, `vault` (pair identifier, 128 bits, unrelated to the name), `side` (`A`/`B`), `name`, `block_size` (1 MiB), `created`, `masks` (description of the mask source), optionally `role` (`backup`) and `media` (spare card identifier).

## Object share `obj/xx/<oid>`

```
"XDK1" | side (1 B) | vault (16 B) | oid (16 B) | length (uint32 BE) | data | SHA-256(everything before)
```

- An object is a file block (up to 1 MiB) or the index. Side A: `M ⊕ R`, side B: `R`. The mask `R` is new for every object.
- Objects are immutable: every write creates a new `oid` (random, not a content hash).
- The checksum covers only its own share, never the other one (D-05, I-03).

## Index (XOR-split object)

JSON: `e` (path → entry: `i` file number, `d` directory, `s` size, `t` time, `b` list of block oids), `n` (next file number), `s` (snapshots: `id`, `q` generation, `x` index oid, `t` time, `p` point pins: `m` medium, `p` point, `r` READY, `t` time).

Paths are canonical (`/a/b`), every entry has a parent directory; an index that does not satisfy this is rejected (S005).

## Superblock `super` (plaintext)

A JSON line `{format, vault, side, cur:{seq, index}, prev:{seq, index}}` and a line with its SHA-256.

## Commit (T-01, A-05)

1. New A and B objects (tmp + fsync + rename).
2. fsync of directories, `F_FULLFSYNC` barrier on both media.
3. Superblock A (prepared state), then superblock B (committed).

On open: A = B → state; A one step ahead of B → completion (when objects are complete) or rollback; other states → explicit error. GC deletes objects unreachable from `cur`, `prev` and all snapshots (B first, then A); with an unreadable snapshot GC is paused.

## Recovery point `points/<id>` (spare card, plaintext)

A JSON line `{format, vault, media, id, snapshot, seq, index, created, objects, bytes, state:"READY"}` and a SHA-256 line. Existence of the record = READY point; it is written after all objects have been persisted and verified. It contains no file names.
