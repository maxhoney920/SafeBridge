// signlatest подписывает latest.json ключом ed25519: помощник проверяет подпись перед
// тем, как доверять хешам из latest.json и ставить обновление.
//
//	go run ./tools/signlatest keygen <файл-для-закрытого-ключа>   # печатает открытый ключ
//	UPDATE_SIGNING_KEY=<base64> go run ./tools/signlatest sign dist/latest.json
//
// Закрытый ключ — 32 байта seed в base64, хранится только в секрете GitHub UPDATE_SIGNING_KEY.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: signlatest keygen <file> | sign <latest.json>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "keygen":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		must(err)
		must(os.WriteFile(os.Args[2], []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600))
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
	case "sign":
		seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("UPDATE_SIGNING_KEY")))
		if err != nil || len(seed) != ed25519.SeedSize {
			fmt.Fprintln(os.Stderr, "UPDATE_SIGNING_KEY: нужен seed ed25519 (32 байта в base64)")
			os.Exit(1)
		}
		priv := ed25519.NewKeyFromSeed(seed)
		b, err := os.ReadFile(os.Args[2])
		must(err)
		sig := ed25519.Sign(priv, b)
		must(os.WriteFile(os.Args[2]+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644))
		fmt.Println("подписан", os.Args[2], "ключом", base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)))
	default:
		os.Exit(2)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
