// Package migrations embeds the reviewed initial database schema. Migration
// authority is separate from application credentials; this package never connects.
package migrations

import _ "embed"

// Initial creates schema version 1 and its roles in a dedicated database cluster.
// Existing-version handling and migration transactions belong to the adapter.
//
//go:embed 001_initial.sql
var Initial string

// Runtime adds schema version 2, applied atomically after version 1.
//
//go:embed 002_runtime.sql
var Runtime string

// ReaderScopes adds schema version 3 with explicit scoped restricted readers.
//
//go:embed 003_reader_scopes.sql
var ReaderScopes string

// Models adds version 4 durable reservations and restricted response artifacts.
//
//go:embed 004_models.sql
var Models string
