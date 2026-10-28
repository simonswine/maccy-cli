# maccy-cli

A small urfave/cli-based macOS CLI for Maccy, loosely following clipman's subcommand interface. Lists and selects entries read-only; checksum-based cleanup can delete entries.

```sh
go build -o maccy-cli .
./maccy-cli list
./maccy-cli --limit 100 list
./maccy-cli --json list
./maccy-cli --limit 0 pick -t fzf
./maccy-cli pick -t fzf --err-on-no-selection
```

Defaults to `~/Library/Containers/org.p0deje.Maccy/Data/Library/Application Support/Maccy/Storage.sqlite`. Override with `--db PATH`.

`list` (also the default command) displays recent entries, including non-text item titles. `--json` includes full plain-text content; treat output as sensitive.

`pick` requires fzf (or a compatible executable that returns one selected input line) and macOS `pbcopy`. It displays escaped, single-line text previews, then copies the original unmodified text. Cancellation succeeds unless `--err-on-no-selection` is set. The default limit is 20; use `--limit 0` for all history.

### Password cleanup

```sh
# Supply the hash, not the password, on stdin (without a trailing newline).
printf '%s' "$checksum" | ./maccy-cli gopass-clear-handler
printf '%s' "$checksum" | ./maccy-cli gopass-clear-handler --limit 20
```

`gopass-clear-handler` accepts an Argon2id PHC hash with an optional `{ARGON2ID}` prefix. It checks the ten newest entries by default, deletes matching text entries and their content, then empties the clipboard using `pbcopy` only if `pbpaste` still matches. Matching preserves whitespace exactly. Hash parameters are bounded (256 MiB memory, 10 iterations) to limit resource use.

**Warning:** Cleanup writes directly to Maccy's Core Data database. Quit Maccy first and back up the database: live changes may not be reflected in its in-memory state or may be overwritten. Deletion is not secure erasure (SQLite journals, backups, and other clipboard managers may retain data). Clipboard read/clear is not atomic; a concurrent clipboard update can be lost.

Unlike clipman, there is no `store` or `restore`: Maccy manages persistence. Picking changes the system clipboard, which Maccy may record normally.
