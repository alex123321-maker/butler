package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")
}
func loopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	return err == nil && net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}
func exclusive(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
func load(path string) (config, error) {
	var cfg config
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return cfg, errors.New("invalid private fixture size")
	}
	d := json.NewDecoder(io.LimitReader(f, 16385))
	d.DisallowUnknownFields()
	if d.Decode(&cfg) != nil || d.Decode(new(any)) != io.EOF || cfg.Username == "" || len(cfg.Username) > 128 || cfg.Password == "" || len(cfg.Password) > 4096 || cfg.PrivateFact == "" || len(cfg.PrivateFact) > 256 {
		return config{}, errors.New("invalid private fixture")
	}
	return cfg, nil
}
func run(args []string) error {
	fs := flag.NewFlagSet("browser-sso-eval", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	init := fs.Bool("init", false, "create a new private synthetic configuration")
	configPath := fs.String("config", "", "private configuration file")
	journalPath := fs.String("journal", "", "new private recipient journal")
	rpListen := fs.String("rp-listen", "127.0.0.1:18098", "loopback RP listener")
	idpListen := fs.String("idp-listen", "127.0.0.1:18099", "loopback provider listener")
	rpOrigin := fs.String("rp-origin", "", "externally visible RP origin")
	idpOrigin := fs.String("idp-origin", "", "externally visible provider origin")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *configPath == "" {
		return errors.New("invalid fixture arguments")
	}
	if *init {
		data, _ := json.Marshal(config{Username: "sso-user-" + opaque(), Password: opaque(), PrivateFact: "SSO-" + opaque()})
		return exclusive(*configPath, append(data, '\n'))
	}
	if !loopback(*rpListen) || !loopback(*idpListen) || *rpListen == *idpListen || !validOrigin(*rpOrigin) || !validOrigin(*idpOrigin) || strings.EqualFold(*rpOrigin, *idpOrigin) || *journalPath == "" {
		return errors.New("invalid fixture topology")
	}
	cfg, err := load(*configPath)
	if err != nil {
		return err
	}
	rp, err := net.Listen("tcp", *rpListen)
	if err != nil {
		return err
	}
	defer rp.Close()
	idp, err := net.Listen("tcp", *idpListen)
	if err != nil {
		return err
	}
	defer idp.Close()
	journal, err := os.OpenFile(*journalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer journal.Close()
	f := newFixture(cfg, *rpOrigin, *idpOrigin, journal)
	server := func(handler http.Handler) *http.Server {
		return &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 8 * time.Second, WriteTimeout: 8 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0)}
	}
	a, b := server(f.handler(false)), server(f.handler(true))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errs := make(chan error, 2)
	go func() { errs <- a.Serve(rp) }()
	go func() { errs <- b.Serve(idp) }()
	select {
	case err = <-errs:
	case <-ctx.Done():
		err = nil
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = a.Shutdown(shutdown)
	_ = b.Shutdown(shutdown)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func main() {
	if run(os.Args[1:]) != nil {
		fmt.Fprintln(os.Stderr, "SSO fixture failed; inspect private configuration and listener settings")
		os.Exit(1)
	}
}
