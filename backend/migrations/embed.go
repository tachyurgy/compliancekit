// Package migrations embeds the SQLite schema so the server binary can apply
// it on boot (see internal/migrate). The .sql files stay the source of truth;
// embedding them means the deployed artifact is still a single static binary.
package migrations

import "embed"

// FS holds every forward migration, named NNNNNN_description.up.sql.
//
//go:embed *.up.sql
var FS embed.FS
