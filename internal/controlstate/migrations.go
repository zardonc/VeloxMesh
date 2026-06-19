package controlstate

import (
	"embed"
)

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

// GetPostgreSQLMigrations returns the embedded PostgreSQL migration files.
func GetPostgreSQLMigrations() embed.FS {
	return postgresMigrations
}

// GetSQLiteMigrations returns the embedded SQLite migration files.
func GetSQLiteMigrations() embed.FS {
	return sqliteMigrations
}
