// Command hws-api hosts the authenticated management/replay surface. Simulation
// execution is injected by an embedding host; no fake/default policy is chosen.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/postgres"
	"github.com/tushardhara/dream/adapters/telemetry"
	api "github.com/tushardhara/dream/adapters/transport"
	"github.com/tushardhara/dream/app/hws"
)

type configuration struct {
	Version        int              `json:"version"`
	Mode           string           `json:"mode"`
	GRPCAddress    string           `json:"grpc_address"`
	HTTPAddress    string           `json:"http_address"`
	Development    bool             `json:"development"`
	TLSCertificate string           `json:"tls_certificate"`
	TLSKey         string           `json:"tls_key"`
	Credentials    []api.Credential `json:"credentials"`
	Grants         []hws.ViewGrant  `json:"grants"`
}
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
func decodeConfiguration(reader io.Reader) (configuration, error) {
	var c configuration
	raw, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return c, api.ErrDenied
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return c, api.ErrDenied
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, api.ErrDenied
	}
	if c.Version != 1 || c.Mode != "management-only" || len(c.Grants) == 0 || len(c.Grants) > 128 {
		return c, api.ErrDenied
	}
	if _, err = api.NewAuthenticator(c.Credentials, time.Now); err != nil {
		return c, api.ErrDenied
	}
	for _, address := range []string{c.GRPCAddress, c.HTTPAddress} {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return c, api.ErrDenied
		}
		ip := net.ParseIP(host)
		if ip == nil || c.Development && !ip.IsLoopback() {
			return c, api.ErrDenied
		}
	}
	if !c.Development && (c.TLSCertificate == "" || c.TLSKey == "") {
		return c, api.ErrDenied
	}
	if (c.TLSCertificate == "") != (c.TLSKey == "") {
		return c, api.ErrDenied
	}
	return c, nil
}
func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("hws-api", flag.ContinueOnError)
	flags.SetOutput(errOut)
	configPath := flags.String("config", "", "trusted JSON configuration file")
	flags.Usage = func() {
		fmt.Fprintln(errOut, "Usage: hws-api --config <file>\nAuthenticated management/replay API. Requires DREAM_DATABASE_URL for a non-owner runtime role.\nExplicit management-only mode never substitutes a fake simulation handler. Step/run-until require a configured execution host.\nDevelopment is explicit and loopback-only; other listeners require configured TLS. No migrations, credentials or provider calls are created.")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *configPath == "" || flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	file, err := os.Open(*configPath)
	if err != nil {
		fmt.Fprintln(errOut, "hws-api: configuration unavailable")
		return 1
	}
	c, err := decodeConfiguration(file)
	_ = file.Close()
	if err != nil {
		fmt.Fprintln(errOut, "hws-api: invalid configuration")
		return 1
	}
	dsn := os.Getenv("DREAM_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(errOut, "hws-api: runtime database configuration required")
		return 1
	}
	var protection *tls.Config
	if c.TLSCertificate != "" {
		certificate, err := tls.LoadX509KeyPair(c.TLSCertificate, c.TLSKey)
		if err != nil {
			fmt.Fprintln(errOut, "hws-api: TLS configuration unavailable")
			return 1
		}
		protection = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}
	}
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil || (!c.Development && !postgres.ProtectedConnection(poolConfig.ConnConfig)) {
		fmt.Fprintln(errOut, "hws-api: protected runtime database configuration required")
		return 1
	}
	poolConfig.MaxConns = 16
	pool, err := pgxpool.NewWithConfig(startup, poolConfig)
	if err != nil {
		fmt.Fprintln(errOut, "hws-api: runtime database unavailable")
		return 1
	}
	defer pool.Close()
	store := postgres.New(pool)
	if err = store.RuntimeReady(startup); err != nil {
		fmt.Fprintln(errOut, "hws-api: runtime database role rejected")
		return 1
	}
	auth, err := api.NewAuthenticator(c.Credentials, time.Now)
	if err != nil {
		return 1
	}
	views, err := hws.NewViewService(store, store, systemClock{}, c.Grants, store)
	if err != nil {
		fmt.Fprintln(errOut, "hws-api: view configuration rejected")
		return 1
	}
	backend := &api.Backend{Auth: auth, Store: store, Views: views}
	signals := &telemetry.Recorder{}
	defer func() { fmt.Fprintf(out, "hws-api: operational_counts=%v\n", signals.Counts()) }()
	servers, err := api.NewServers(startup, api.ServerConfig{Observer: signals, Auth: auth, Backend: backend, Development: c.Development, TLS: protection, RuntimeReady: store.RuntimeReady, Admission: func(ctx context.Context, c api.Credential) error {
		return store.AdmitRequest(ctx, hws.RequestBudget{Credential: c.ID, Scope: c.Scope, PerMinute: c.RequestsPerMinute, Total: c.TotalRequests})
	}})
	if err != nil {
		fmt.Fprintln(errOut, "hws-api: server configuration rejected")
		return 1
	}
	defer servers.Close()
	rpc, err := net.Listen("tcp", c.GRPCAddress)
	if err != nil {
		fmt.Fprintln(errOut, "hws-api: listener unavailable")
		return 1
	}
	defer rpc.Close()
	http, err := net.Listen("tcp", c.HTTPAddress)
	if err != nil {
		fmt.Fprintln(errOut, "hws-api: listener unavailable")
		return 1
	}
	defer http.Close()
	failures := make(chan error, 2)
	go func() { failures <- servers.ServeGRPC(rpc) }()
	go func() { failures <- servers.ServeHTTP(http) }()
	fmt.Fprintf(out, "hws-api: management-only grpc=%s http=%s\n", rpc.Addr(), http.Addr())
	select {
	case <-ctx.Done():
		drain, cancelDrain := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancelDrain()
		if servers.Shutdown(drain) != nil {
			fmt.Fprintln(errOut, "hws-api: drain deadline reached; durable state requires reconciliation")
			return 1
		}
		return 0
	case <-failures:
		fmt.Fprintln(errOut, "hws-api: listener stopped")
		return 1
	}
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
