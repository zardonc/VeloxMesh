package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"veloxmesh/internal/controlstate/migration"
)

func main() {
	sqliteDSN := flag.String("sqlite", os.Getenv("SQLITE_DSN"), "source SQLite DSN or file path")
	postgresDSN := flag.String("postgres", os.Getenv("POSTGRES_DSN"), "target PostgreSQL DSN")
	flag.Parse()

	if *sqliteDSN == "" || *postgresDSN == "" {
		fmt.Fprintln(os.Stderr, "sqlite and postgres DSNs are required")
		os.Exit(2)
	}

	report, err := migration.Migrate(context.Background(), migration.Options{
		SQLiteDSN:   *sqliteDSN,
		PostgresDSN: *postgresDSN,
	})
	data, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(data))
	if err != nil {
		os.Exit(1)
	}
}
