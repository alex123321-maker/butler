// browser-spa-eval is an isolated recipient, never a production policy exception.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/butler/butler/internal/browserspa/eval"
	"github.com/butler/butler/internal/jsonstrict"
)

func opaque() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("random source unavailable")
	}
	return hex.EncodeToString(b[:])
}
func exclusive(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}
func run(args []string) error {
	fs := flag.NewFlagSet("browser-spa-eval", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	init := fs.Bool("init", false, "create private independent expected values")
	config := fs.String("config", "", "private configuration path")
	origin := fs.String("origin", "", "public HTTPS origin, initialization only")
	journal := fs.String("journal", "", "new private recipient journal")
	listen := fs.String("listen", "127.0.0.1:18198", "loopback reverse-proxy listener")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *config == "" {
		return errors.New("invalid fixture arguments")
	}
	if *init {
		c := eval.Config{Origin: *origin, Variant: opaque()[:16], Category: "books", Title: "Catalog " + opaque(), Marker: "Receipt-" + opaque(), Stock: 731}
		if _, err := eval.New(c, func(eval.Event) {}); err != nil {
			return err
		}
		f, err := exclusive(*config)
		if err != nil {
			return errors.New("cannot create private fixture")
		}
		defer f.Close()
		if json.NewEncoder(f).Encode(c) != nil || f.Sync() != nil {
			return errors.New("cannot persist private fixture")
		}
		return nil
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || *journal == "" || *origin != "" {
		return errors.New("invalid isolated listener")
	}
	f, err := os.Open(*config)
	if err != nil {
		return errors.New("private fixture unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return errors.New("invalid private fixture")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	var c eval.Config
	if err != nil || jsonstrict.Decode(raw, &c) != nil {
		return errors.New("invalid private fixture")
	}
	j, err := exclusive(*journal)
	if err != nil {
		return errors.New("cannot create recipient journal")
	}
	defer j.Close()
	var mu sync.Mutex
	// Failed durable recipient observation stops this standalone process. No
	// successful test evidence is manufactured after losing its journal.
	fixture, err := eval.New(c, func(e eval.Event) {
		mu.Lock()
		defer mu.Unlock()
		if json.NewEncoder(j).Encode(e) != nil || j.Sync() != nil {
			log.Fatal("recipient journal unavailable")
		}
	})
	if err != nil {
		return err
	}
	l, err := net.Listen("tcp", *listen)
	if err != nil {
		return errors.New("isolated listener unavailable")
	}
	defer l.Close()
	s := &http.Server{Handler: fixture, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 16384, ErrorLog: log.New(io.Discard, "", 0)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- s.Serve(l) }()
	fmt.Println("isolated SPA recipient ready")
	select {
	case <-ctx.Done():
		_ = s.Close()
		return nil
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("isolated recipient stopped")
	}
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
