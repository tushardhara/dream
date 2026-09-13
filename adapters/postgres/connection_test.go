package postgres

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
)

func TestProtectedDatabaseConnectionCannotFallBackToCleartext(t *testing.T) {
	for _, mode := range []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"} {
		cfg, e := pgx.ParseConfig("postgres://fixture:synthetic@127.0.0.1/fixture?sslmode=" + mode)
		if e != nil {
			t.Fatal(e)
		}
		want := mode == "verify-ca" || mode == "verify-full"
		if ProtectedConnection(cfg) != want {
			t.Fatal(mode, "unexpected transport protection")
		}
	}
	cfg, e := pgx.ParseConfig("postgres://fixture:synthetic@127.0.0.1/fixture?sslmode=verify-full")
	if e != nil {
		t.Fatal(e)
	}
	cfg.Fallbacks = append(cfg.Fallbacks, &pgconn.FallbackConfig{Host: "127.0.0.1", Port: 5432})
	if ProtectedConnection(cfg) {
		t.Fatal("verified primary hid plaintext fallback")
	}

}
