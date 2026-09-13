// Command hws-admin performs explicit operator migrations/restore preparation.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/postgres"
	"io"
	"os"
	"time"
)

func run(ctx context.Context, args []string, out io.Writer) int {
	flags := flag.NewFlagSet("hws-admin", flag.ContinueOnError)
	flags.SetOutput(out)
	development := flags.Bool("development", false, "explicit disposable/local cleartext database")
	file := flags.String("file", "", "revocation journal file; new exports created with mode 0600")
	expected := flags.String("expected-sha256", "", "independently retained newest revocation journal digest")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: hws-admin [--development] [--file path] [--expected-sha256 digest] migrate|export-revocations|quarantine-restore|apply-revocations\nExplicit operator action using DREAM_DATABASE_URL. Isolate/drain restored databases before quarantine; never infer journal freshness from an old backup.")
	}
	if e := flags.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	action := flags.Arg(0)
	if action != "migrate" && action != "export-revocations" && action != "quarantine-restore" && action != "apply-revocations" {
		flags.Usage()
		return 2
	}
	if (action == "export-revocations" || action == "apply-revocations") && *file == "" || (action == "quarantine-restore" && *expected == "") {
		flags.Usage()
		return 2
	}
	if (*expected != "" && action != "quarantine-restore") || (*file != "" && action != "export-revocations" && action != "apply-revocations") {
		flags.Usage()
		return 2
	}
	dsn := os.Getenv("DREAM_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(out, "hws-admin: database configuration required")
		return 1
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || (!*development && !postgres.ProtectedConnection(cfg.ConnConfig)) {
		fmt.Fprintln(out, "hws-admin: protected database configuration required")
		return 1
	}
	cfg.MaxConns = 2
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool, e := pgxpool.NewWithConfig(bounded, cfg)
	if e != nil {
		fmt.Fprintln(out, "hws-admin: database unavailable")
		return 1
	}
	defer pool.Close()
	store := postgres.New(pool)
	switch action {
	case "migrate":
		e = postgres.Migrate(bounded, pool)
	case "quarantine-restore":
		e = store.QuarantineRestore(bounded, *expected)
	case "export-revocations":
		var journal postgres.RevocationJournal
		journal, e = store.ExportRevocations(bounded)
		if e == nil {
			var raw []byte
			raw, e = json.Marshal(journal)
			if e == nil {
				var f *os.File
				f, e = os.OpenFile(*file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if e == nil {
					_, e = f.Write(raw)
					if e == nil {
						e = f.Sync()
					}
					closeErr := f.Close()
					if e == nil {
						e = closeErr
					}
				}
				if e == nil {
					digest, _ := journal.Digest()
					fmt.Fprintln(out, "revocation-journal sha256="+digest)
				}
			}
		}
	case "apply-revocations":
		var f *os.File
		f, e = os.Open(*file)
		if e == nil {
			var raw []byte
			raw, e = io.ReadAll(io.LimitReader(f, (16<<20)+1))
			f.Close()
			if e == nil && len(raw) > 16<<20 {
				e = errors.New("journal byte budget")
			}
			if e == nil {
				var journal postgres.RevocationJournal
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.DisallowUnknownFields()
				e = decoder.Decode(&journal)
				if e == nil && decoder.Decode(new(any)) != io.EOF {
					e = errors.New("invalid journal")
				}
				if e == nil {
					e = store.ApplyRevocationJournal(bounded, journal)
				}
			}
		}
	}
	if e != nil {
		fmt.Fprintln(out, "hws-admin: operation failed; preserve quarantine and inspect operator inputs")
		return 1
	}
	fmt.Fprintln(out, "hws-admin: operation complete")
	return 0
}
func main() { os.Exit(run(context.Background(), os.Args[1:], os.Stdout)) }
