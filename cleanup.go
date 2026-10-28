package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/argon2"
)

// parseChecksum bounds work and memory before accepting an untrusted PHC string.
func parseChecksum(encoded string) (func([]byte) bool, error) {
	invalid := fmt.Errorf("invalid or unsupported Argon2id checksum")
	parts := strings.Split(strings.TrimPrefix(encoded, "{ARGON2ID}"), "$")
	if strings.ContainsAny(encoded, "\r\n") || len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return nil, invalid
	}
	var memory, iterations uint32
	var parallelism uint32
	var trailing string
	n, _ := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d%s", &memory, &iterations, &parallelism, &trailing)
	if n != 3 || fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, parallelism) != parts[3] || parallelism < 1 || parallelism > 255 || memory < 8*parallelism || memory > 256*1024 || iterations < 1 || iterations > 10 {
		return nil, invalid
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 1024 {
		return nil, invalid
	}
	digest, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(digest) < 4 || len(digest) > 1024 {
		return nil, invalid
	}
	return func(value []byte) bool {
		actual := argon2.IDKey(value, salt, iterations, memory, uint8(parallelism), uint32(len(digest)))
		return subtle.ConstantTimeCompare(actual, digest) == 1
	}, nil
}

func cleanupAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Int("limit") <= 0 {
		return fmt.Errorf("gopass-clear-handler limit must be positive")
	}
	encoded, err := io.ReadAll(io.LimitReader(cmd.Reader, 4097))
	if err != nil {
		return err
	}
	if len(encoded) > 4096 {
		return fmt.Errorf("checksum too long")
	}
	matches, err := parseChecksum(string(encoded))
	if err != nil {
		return err
	}
	items, err := loadCommand(ctx, cmd)
	if err != nil {
		return err
	}
	var ids []int64
	for _, item := range items {
		if item.Text != nil && matches([]byte(*item.Text)) {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) > 0 {
		// Resolve the same default path as loadCommand.
		path, err := databasePath(cmd.String("db"))
		if err != nil {
			return err
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		uri := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=rw"}
		db, err := sql.Open("sqlite", uri.String())
		if err != nil {
			return err
		}
		defer db.Close()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, id := range ids {
			// Recheck the exact content in case Maccy changed the entry meanwhile.
			var value []byte
			err := tx.QueryRowContext(ctx, `SELECT ZVALUE FROM ZHISTORYITEMCONTENT WHERE ZITEM = ? AND ZTYPE = 'public.utf8-plain-text' ORDER BY Z_PK LIMIT 1`, id).Scan(&value)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			if !matches(value) {
				continue
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM ZHISTORYITEMCONTENT WHERE ZITEM = ?`, id); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM ZHISTORYITEM WHERE Z_PK = ?`, id); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	current, err := exec.CommandContext(ctx, "pbpaste").Output()
	if err != nil {
		return fmt.Errorf("pbpaste: %w", err)
	}
	// Read/clear is not atomic: a concurrent clipboard update can still be lost.
	if matches(current) {
		if err := exec.CommandContext(ctx, "pbcopy").Run(); err != nil {
			return fmt.Errorf("pbcopy: %w", err)
		}
	}
	return nil
}
