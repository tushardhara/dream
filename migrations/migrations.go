// Package migrations embeds the reviewed initial database schema. Migration
// authority is separate from application credentials; this package never connects.
package migrations

import _ "embed"

// Initial creates schema version 1 and its roles in a dedicated database cluster.
// Existing-version handling and migration transactions belong to the adapter.
//
//go:embed 001_initial.sql
var Initial string
