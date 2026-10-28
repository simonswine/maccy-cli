package main

import (
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestChecksum(t *testing.T) {
	salt := []byte("test salt")
	digest := argon2.IDKey([]byte("secret\n"), salt, 1, 32, 1, 32)
	hash := "$argon2id$v=19$m=32,t=1,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(digest)
	for _, prefix := range []string{"", "{ARGON2ID}"} {
		matches, err := parseChecksum(prefix + hash)
		if err != nil {
			t.Fatal(err)
		}
		if !matches([]byte("secret\n")) || matches([]byte("secret")) || matches(nil) {
			t.Fatal("incorrect match")
		}
	}
	for _, bad := range []string{"", hash + "\n", "$argon2id$v=19$m=999999999,t=1,p=1$abcdefgh$abcd", "$argon2id$v=19$m=32,t=0,p=1$abcdefgh$abcd"} {
		if _, err := parseChecksum(bad); err == nil {
			t.Fatalf("accepted invalid checksum %q", bad)
		}
	}
}
