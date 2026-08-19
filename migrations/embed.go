// Package migrations embeds the SQL migration files so integration tests can
// provision isolated per-package schemas without depending on the golang-migrate
// CLI or a pre-migrated database.
package migrations

import "embed"

// Files contains every .sql file from migrations/ keyed by file name. The
// migrate CLI keeps reading the directory on disk (it ignores .go files), so
// embedding is additive and does not change deployment workflows.
//
//go:embed *.sql
var Files embed.FS
