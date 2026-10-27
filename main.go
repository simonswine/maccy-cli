package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"
	_ "modernc.org/sqlite"
)

type item struct {
	ID          int64     `json:"id"`
	CopiedAt    time.Time `json:"copied_at"`
	Application string    `json:"application"`
	Title       string    `json:"title"`
	Text        *string   `json:"text,omitempty"`
}

func main() {
	if err := command().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func command() *cli.Command {
	return &cli.Command{
		Name:  "maccy-cli",
		Usage: "List Maccy clipboard history (read-only)",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Usage: "Path to Maccy's Storage.sqlite"},
			&cli.IntFlag{Name: "limit", Aliases: []string{"n"}, Value: 20, Usage: "Maximum entries (0 for all)"},
			&cli.BoolFlag{Name: "json", Usage: "Output JSON, including full plain-text content"},
		},
		Commands: []*cli.Command{
			{Name: "list", Usage: "List clipboard history", Action: listAction},
			{Name: "pick", Usage: "Select plain text and copy it to the clipboard", Flags: []cli.Flag{
				&cli.StringFlag{Name: "tool", Aliases: []string{"t"}, Value: "fzf", Usage: "Selector executable (fzf-compatible line selection)"},
				&cli.BoolFlag{Name: "err-on-no-selection", Usage: "Return an error when selection is cancelled"},
			}, Action: pickAction},
		},
		Action: listAction,
	}
}

func listAction(ctx context.Context, cmd *cli.Command) error {
	items, err := loadCommand(ctx, cmd)
	if err != nil {
		return err
	}
	return output(cmd.Writer, items, cmd.Bool("json"))
}

func loadCommand(ctx context.Context, cmd *cli.Command) ([]item, error) {
	if cmd.Int("limit") < 0 {
		return nil, fmt.Errorf("limit must be nonnegative")
	}
	path := cmd.String("db")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, "Library/Containers/org.p0deje.Maccy/Data/Library/Application Support/Maccy/Storage.sqlite")
	}
	return load(ctx, path, cmd.Int("limit"))
}

func pickAction(ctx context.Context, cmd *cli.Command) error {
	items, err := loadCommand(ctx, cmd)
	if err != nil {
		return err
	}
	var input strings.Builder
	texts := make(map[int64]string)
	for _, i := range items {
		if i.Text == nil {
			continue
		}
		texts[i.ID] = *i.Text
		fmt.Fprintf(&input, "%d\t%q\n", i.ID, *i.Text)
	}
	if len(texts) == 0 {
		if cmd.Bool("err-on-no-selection") {
			return fmt.Errorf("no text entries")
		}
		return nil
	}
	selector := exec.CommandContext(ctx, cmd.String("tool"))
	selector.Stdin = strings.NewReader(input.String())
	selector.Stderr = os.Stderr
	selected, err := selector.Output()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if exit, ok := err.(*exec.ExitError); ok && (exit.ExitCode() == 1 || exit.ExitCode() == 130) && !cmd.Bool("err-on-no-selection") {
			return nil
		}
		return fmt.Errorf("selector: %w", err)
	}
	line := strings.TrimSpace(string(selected))
	if line == "" && !cmd.Bool("err-on-no-selection") {
		return nil
	}
	id, err := strconv.ParseInt(strings.SplitN(line, "\t", 2)[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid selection")
	}
	text, ok := texts[id]
	if !ok {
		return fmt.Errorf("unknown selection")
	}
	copy := exec.CommandContext(ctx, "pbcopy")
	copy.Stdin = strings.NewReader(text)
	if err := copy.Run(); err != nil {
		return fmt.Errorf("pbcopy: %w", err)
	}
	return nil
}

func load(ctx context.Context, path string, limit int) ([]item, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	n := limit
	if n == 0 {
		n = -1
	}
	rows, err := db.QueryContext(ctx, `SELECT h.Z_PK, COALESCE(h.ZLASTCOPIEDAT, 0),
 COALESCE(h.ZAPPLICATION, ''), COALESCE(h.ZTITLE, ''),
 (SELECT CAST(c.ZVALUE AS TEXT) FROM ZHISTORYITEMCONTENT c
 WHERE c.ZITEM = h.Z_PK AND c.ZTYPE = 'public.utf8-plain-text'
 ORDER BY c.Z_PK LIMIT 1)
 FROM ZHISTORYITEM h ORDER BY h.ZLASTCOPIEDAT DESC, h.Z_PK DESC LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("read Maccy database: %w", err)
	}
	defer rows.Close()
	items := make([]item, 0)
	for rows.Next() {
		var i item
		var seconds float64
		if err := rows.Scan(&i.ID, &seconds, &i.Application, &i.Title, &i.Text); err != nil {
			return nil, err
		}
		// Core Data timestamps count seconds since 2001-01-01 UTC.
		i.CopiedAt = time.Unix(978307200, 0).Add(time.Duration(seconds * float64(time.Second))).UTC()
		items = append(items, i)
	}
	return items, rows.Err()
}

func output(w io.Writer, items []item, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tCOPIED AT\tAPPLICATION\tTITLE"); err != nil {
		return err
	}
	for _, i := range items {
		// Quote clipboard-controlled strings to avoid terminal escape injection.
		title := i.Title
		if title == "" && i.Text != nil {
			title = *i.Text
		}
		title = strings.Join(strings.Fields(title), " ")
		runes := []rune(title)
		if len(runes) > 120 {
			title = string(runes[:120]) + "…"
		}
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%q\t%q\n", i.ID, i.CopiedAt.Local().Format(time.RFC3339), i.Application, title); err != nil {
			return err
		}
	}
	return tw.Flush()
}
