// Standalone launcher for the Xeme Ledger REST server.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/xeme-os/xeme/internal/ledger"
)

func main() {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".xeme", "ledger.db")
	if v := os.Getenv("XEME_LEDGER_DB"); v != "" {
		dbPath = v
	}
	addr := ":8088"
	if v := os.Getenv("XEME_LEDGER_ADDR"); v != "" {
		addr = v
	}

	l, err := ledger.Open(dbPath)
	if err != nil {
		log.Fatalf("open ledger: %v", err)
	}
	defer l.Close()

	fmt.Printf("═══════════════════════════════════════════════════════════\n")
	fmt.Printf("  Xeme Ledger — local SQLite CRM\n")
	fmt.Printf("═══════════════════════════════════════════════════════════\n")
	fmt.Printf("  DB:    %s\n", l.Path())
	fmt.Printf("  HTTP:  http://localhost%s\n", addr)
	fmt.Printf("\n")
	fmt.Printf("  Endpoints:\n")
	fmt.Printf("    GET  /health\n")
	fmt.Printf("    GET  /v1/contacts?q=&min_score=&limit=\n")
	fmt.Printf("    POST /v1/contacts\n")
	fmt.Printf("    GET  /v1/contacts/{id}\n")
	fmt.Printf("    GET  /v1/companies\n")
	fmt.Printf("    POST /v1/outcomes\n")
	fmt.Printf("    GET  /v1/stats\n")
	fmt.Printf("\n")
	if err := l.ServeHTTP(addr); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
