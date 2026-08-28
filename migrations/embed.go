// Package migrations embeds the raw SQL migration files so they can be
// applied at startup by the store without relying on filesystem layout.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
