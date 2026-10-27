# maccy-cli

A small urfave/cli-based macOS CLI for Maccy, loosely following clipman's subcommand interface. Opens Maccy's Core Data SQLite database read-only.

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

Unlike clipman, there is no `store`, `clear`, or `restore`: Maccy manages persistence, and this tool does not write to its database. Picking changes the system clipboard, which Maccy may record normally.
