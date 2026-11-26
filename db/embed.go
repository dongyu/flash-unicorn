package db

import "embed"

//go:embed postgres/migrations/*.sql
var PostgresMigrations embed.FS

//go:embed sqlite/migrations/*.sql
var SQLiteMigrations embed.FS
