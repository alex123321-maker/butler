package main

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/butler/butler/apps/browser-bridge/internal/app"
	"github.com/butler/butler/internal/resourceexec"
)

func main() {
	// Only operator-fixed paths/targets are accepted. No inline token or tool arg
	// config, ambient proxy, raw HTTP control endpoint or legacy server exists.
	file := os.Getenv("BUTLER_GUARDED_BROWSER_TOKEN_FILE")
	data, e := readPrivateToken(file)
	if e != nil {
		os.Exit(2)
	}
	token := strings.TrimSpace(string(data))
	authority, e := resourceexec.NewHTTPAuthority(os.Getenv("BUTLER_GUARDED_BROWSER_AUTHORITY_URL"), token)
	if e != nil {
		os.Exit(2)
	}
	addr := os.Getenv("BUTLER_GUARDED_BROWSER_GRPC_ADDR")
	if addr == "" {
		addr = "127.0.0.1:19090"
	}
	if guardedListenAddress(addr) != nil {
		os.Exit(2)
	}
	listener, e := net.Listen("tcp", addr)
	if e != nil {
		os.Exit(2)
	}
	defer listener.Close()
	var metadata net.Listener
	if metadataAddr := os.Getenv("BUTLER_GUARDED_BROWSER_METADATA_ADDR"); metadataAddr != "" {
		if guardedListenAddress(metadataAddr) != nil {
			os.Exit(2)
		}
		metadata, e = net.Listen("tcp", metadataAddr)
		if e != nil {
			os.Exit(2)
		}
		defer metadata.Close()
	}
	if app.RunGuardedWithMetadata(context.Background(), os.Stdin, os.Stdout, listener, metadata, authority, token) != nil {
		os.Exit(1)
	}
}

func guardedListenAddress(addr string) error {
	host, port, e := net.SplitHostPort(addr)
	ip := net.ParseIP(host)
	n, portErr := strconv.Atoi(port)
	if e != nil || ip == nil || !ip.IsLoopback() || portErr != nil || n < 1 || n > 65535 {
		return resourceexec.ErrDenied
	}
	return nil
}

func readPrivateToken(path string) ([]byte, error) {
	root, e := os.OpenRoot(filepath.Dir(path))
	if e != nil {
		return nil, e
	}
	defer root.Close()
	name := filepath.Base(path)
	before, e := root.Lstat(name)
	if e != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 {
		return nil, resourceexec.ErrDenied
	}
	file, e := root.Open(name)
	if e != nil {
		return nil, e
	}
	defer file.Close()
	actual, e := file.Stat()
	if e != nil || !actual.Mode().IsRegular() || actual.Mode().Perm()&0077 != 0 || actual.Size() > 4096 || !os.SameFile(before, actual) {
		return nil, resourceexec.ErrDenied
	}
	data, e := io.ReadAll(io.LimitReader(file, 4097))
	if e != nil || len(data) > 4096 {
		return nil, resourceexec.ErrDenied
	}
	return data, nil
}
