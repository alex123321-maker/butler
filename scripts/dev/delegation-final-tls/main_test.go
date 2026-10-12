package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testCertificate(t *testing.T, hostname string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: hostname}, DNSNames: []string{hostname}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots
}

func successfulLookup(_ context.Context, host string) ([]net.IPAddr, error) {
	if host != targetHost && host != proxyHost {
		return nil, errors.New("unexpected lookup")
	}
	return []net.IPAddr{{IP: net.IPv4(127, 0, 0, 1)}}, nil
}

func TestFinalTLSPrerequisiteConcurrentAuthlessTunnel(t *testing.T) {
	cert, roots := testCertificate(t, targetHost)
	var entries, handshakes, applicationBytes atomic.Int32
	var servers sync.WaitGroup
	bothDialed := make(chan struct{})
	errorsSeen := make(chan error, 2)
	deps := probeDependencies{lookup: successfulLookup, tlsConfig: &tls.Config{RootCAs: roots}}
	deps.dial = func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != proxyAddress {
			return nil, errors.New("unexpected destination")
		}
		client, server := net.Pipe()
		if entries.Add(1) == 2 {
			close(bothDialed)
		}
		servers.Add(1)
		go func() {
			defer servers.Done()
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(3 * time.Second))
			request, err := http.ReadRequest(bufio.NewReader(server))
			if err != nil {
				errorsSeen <- err
				return
			}
			if request.Method != "CONNECT" || request.Host != targetAddress || request.RequestURI != targetAddress || len(request.Header) != 0 {
				errorsSeen <- errors.New("CONNECT must contain only fixed authority, without authentication")
				return
			}
			// Neither lane receives a tunnel until both are concurrently open.
			select {
			case <-bothDialed:
			case <-time.After(time.Second):
				errorsSeen <- errors.New("lanes serialized")
				return
			}
			if _, err := io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
				errorsSeen <- err
				return
			}
			secured := tls.Server(server, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
			if err := secured.Handshake(); err != nil {
				errorsSeen <- err
				return
			}
			if secured.ConnectionState().ServerName != targetHost {
				errorsSeen <- errors.New("unexpected SNI")
				return
			}
			handshakes.Add(1)
			var body [1]byte
			n, _ := secured.Read(body[:])
			applicationBytes.Add(int32(n))
		}()
		return client, nil
	}
	report := runProbe(context.Background(), deps)
	servers.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Error(err)
	}
	if report.Status != "passed" || entries.Load() != 2 || handshakes.Load() != 2 || applicationBytes.Load() != 0 {
		t.Fatalf("concurrent TLS-only proof failed: %#v entries=%d handshakes=%d bytes=%d", report, entries.Load(), handshakes.Load(), applicationBytes.Load())
	}
	if report.AuthenticationUsed || report.ProviderHTTPRequests != 0 || report.NativeAttempts != 0 || report.MaximumSeconds != 15 {
		t.Fatalf("unexpected authority: %#v", report)
	}
}

func TestFinalTLSPrerequisiteDeniesIncompleteConnections(t *testing.T) {
	for _, test := range []struct{ name, response, expected string }{
		{"proxy_reject", "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: private-secret\r\n\r\n", "connect_denied"},
		{"oversized_headers", "HTTP/1.1 200 Connection Established\r\nX-Private: " + strings.Repeat("private-secret", maximumConnectResponseBytes) + "\r\n\r\n", "connect_denied"},
		{"surplus_tunnel_bytes", "HTTP/1.1 200 Connection Established\r\n\r\nprivate-secret", "connect_denied"},
		{"invalid_tls", "HTTP/1.1 200 Connection Established\r\n\r\n", "tls_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var entries atomic.Int32
			deps := probeDependencies{lookup: successfulLookup, tlsConfig: &tls.Config{}}
			deps.dial = func(context.Context, string, string) (net.Conn, error) {
				entries.Add(1)
				client, server := net.Pipe()
				go func() {
					defer server.Close()
					_ = server.SetDeadline(time.Now().Add(time.Second))
					_, _ = http.ReadRequest(bufio.NewReader(server))
					_, _ = io.WriteString(server, test.response)
					if test.name == "invalid_tls" {
						var body [1024]byte
						_, _ = server.Read(body[:])
						_, _ = io.WriteString(server, "not TLS private-secret")
					}
				}()
				return client, nil
			}
			report := runProbe(context.Background(), deps)
			if report.Status != "denied" || entries.Load() != 2 {
				t.Fatalf("denial/retry boundary failed: %#v", report)
			}
			for _, lane := range report.Lanes {
				if lane.TLS || lane.Failure != test.expected {
					t.Fatalf("unexpected failure: %#v", lane)
				}
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "private-secret") {
				t.Fatal("private error/header escaped safe evidence")
			}
		})
	}
}

func TestFinalTLSPrerequisiteVerifiesHostnameAndTrust(t *testing.T) {
	for _, hostname := range []string{"wrong.example", targetHost} {
		t.Run(hostname, func(t *testing.T) {
			cert, roots := testCertificate(t, hostname)
			if hostname == targetHost {
				roots = x509.NewCertPool()
			}
			deps := probeDependencies{lookup: successfulLookup, tlsConfig: &tls.Config{RootCAs: roots, InsecureSkipVerify: true, ServerName: hostname}}
			deps.dial = func(context.Context, string, string) (net.Conn, error) {
				client, server := net.Pipe()
				go func() {
					defer server.Close()
					_ = server.SetDeadline(time.Now().Add(time.Second))
					_, _ = http.ReadRequest(bufio.NewReader(server))
					_, _ = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\n")
					_ = tls.Server(server, &tls.Config{Certificates: []tls.Certificate{cert}}).Handshake()
				}()
				return client, nil
			}
			report := runProbe(context.Background(), deps)
			if report.Status != "denied" {
				t.Fatalf("untrusted hostname accepted: %#v", report)
			}
			for _, lane := range report.Lanes {
				if lane.TLS || lane.Failure != "tls_failed" {
					t.Fatalf("hostname/trust guard missing: %#v", lane)
				}
			}
		})
	}
}

func TestFinalTLSPrerequisiteSingleFailedLaneDeniesWithoutRetry(t *testing.T) {
	cert, roots := testCertificate(t, targetHost)
	var entries atomic.Int32
	deps := probeDependencies{lookup: successfulLookup, tlsConfig: &tls.Config{RootCAs: roots}}
	deps.dial = func(context.Context, string, string) (net.Conn, error) {
		if entries.Add(1) == 1 {
			return nil, errors.New("private socket diagnostic")
		}
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(time.Second))
			_, _ = http.ReadRequest(bufio.NewReader(server))
			_, _ = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\n")
			secured := tls.Server(server, &tls.Config{Certificates: []tls.Certificate{cert}})
			if secured.Handshake() == nil {
				var body [1]byte
				_, _ = secured.Read(body[:])
			}
		}()
		return client, nil
	}
	report := runProbe(context.Background(), deps)
	passed := 0
	for _, lane := range report.Lanes {
		if lane.TLS {
			passed++
		}
	}
	if report.Status != "denied" || passed != 1 || entries.Load() != 2 {
		t.Fatalf("partial proof accepted/retried: %#v", report)
	}
}

func TestFinalTLSPrerequisiteDNSAndDeadlineFailClosed(t *testing.T) {
	for _, name := range []string{"target_dns", "proxy_dns", "deadline"} {
		t.Run(name, func(t *testing.T) {
			var entries atomic.Int32
			deps := probeDependencies{tlsConfig: &tls.Config{}}
			deps.lookup = func(ctx context.Context, host string) ([]net.IPAddr, error) {
				if name == "deadline" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				if name == "target_dns" || host == proxyHost {
					return nil, errors.New("private DNS result")
				}
				return successfulLookup(ctx, host)
			}
			deps.dial = func(context.Context, string, string) (net.Conn, error) {
				entries.Add(1)
				return nil, errors.New("must not connect")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			report := runProbe(ctx, deps)
			if report.Status != "denied" || entries.Load() != 0 {
				t.Fatalf("DNS/deadline denial missing: %#v", report)
			}
		})
	}
}
