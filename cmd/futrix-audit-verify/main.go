package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/FutrixDev/FutrixPackage/pkg/auditchain"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: futrix-audit-verify <audit.jsonl>")
		os.Exit(2)
	}
	result, err := auditchain.VerifyFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !result.Pass {
		os.Exit(1)
	}
}
