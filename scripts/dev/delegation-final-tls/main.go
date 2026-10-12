// Command delegation-final-tls is a fixed, credential-free prerequisite for the
// separately bounded final delegation fixture. It never sends provider HTTP.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	targetHost                  = "daily-cloudcode-pa.googleapis.com"
	targetAddress               = targetHost + ":443"
	proxyHost                   = "antigravity-proxy"
	proxyAddress                = proxyHost + ":18080"
	probeTimeout                = 15 * time.Second
	maximumConnectResponseBytes = 8192
)

type laneEvidence struct {
	Lane      int    `json:"lane"`
	DNS       bool   `json:"dns_resolved"`
	ProxyDNS  bool   `json:"proxy_dns_resolved"`
	Connected bool   `json:"proxy_connected"`
	Tunnel    bool   `json:"connect_accepted"`
	TLS       bool   `json:"tls_hostname_verified"`
	Failure   string `json:"failure,omitempty"`
}

type probeEvidence struct {
	Status               string         `json:"status"`
	Target               string         `json:"target"`
	Lanes                []laneEvidence `json:"lanes"`
	MaximumSeconds       int            `json:"maximum_seconds"`
	AuthenticationUsed   bool           `json:"authentication_used"`
	ProviderHTTPRequests int            `json:"provider_http_requests"`
	NativeAttempts       int            `json:"native_attempts"`
}

// Dependencies are private test seams. No CLI/environment/configuration path
// can replace the fixed production destinations or TLS verification policy.
type probeDependencies struct {
	lookup    func(context.Context, string) ([]net.IPAddr, error)
	dial      func(context.Context, string, string) (net.Conn, error)
	tlsConfig *tls.Config
}

func defaultDependencies() probeDependencies {
	return probeDependencies{
		lookup:    net.DefaultResolver.LookupIPAddr,
		dial:      (&net.Dialer{}).DialContext,
		tlsConfig: &tls.Config{ServerName: targetHost, MinVersion: tls.VersionTLS12},
	}
}

func probeLane(ctx context.Context, lane int, deps probeDependencies) laneEvidence {
	e := laneEvidence{Lane: lane}
	addresses, err := deps.lookup(ctx, targetHost)
	if err != nil || len(addresses) == 0 {
		e.Failure = "dns_failed"
		return e
	}
	e.DNS = true
	addresses, err = deps.lookup(ctx, proxyHost)
	if err != nil || len(addresses) == 0 {
		e.Failure = "proxy_dns_failed"
		return e
	}
	e.ProxyDNS = true
	conn, err := deps.dial(ctx, "tcp", proxyAddress)
	if err != nil {
		e.Failure = "proxy_connect_failed"
		return e
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); !ok || conn.SetDeadline(deadline) != nil {
		e.Failure = "deadline_failed"
		return e
	}
	e.Connected = true
	// CONNECT is only tunnel setup. No proxy authentication, cookies, token,
	// project ID, import file or provider request is available to this command.
	request := "CONNECT " + targetAddress + " HTTP/1.1\r\nHost: " + targetAddress + "\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		e.Failure = "connect_write_failed"
		return e
	}
	reader := bufio.NewReader(io.LimitReader(conn, maximumConnectResponseBytes))
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
	if err != nil || response.StatusCode != http.StatusOK || reader.Buffered() != 0 {
		e.Failure = "connect_denied"
		return e
	}
	e.Tunnel = true
	config := deps.tlsConfig.Clone()
	config.ServerName = targetHost
	config.MinVersion = tls.VersionTLS12
	config.InsecureSkipVerify = false
	secured := tls.Client(conn, config)
	if err := secured.HandshakeContext(ctx); err != nil {
		e.Failure = "tls_failed"
		return e
	}
	e.TLS = true
	// No application write follows this handshake. Closing the socket sends no
	// authenticated/provider HTTP and does not consume a model admission slot.
	return e
}

func runProbe(parent context.Context, deps probeDependencies) probeEvidence {
	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()
	report := probeEvidence{Status: "denied", Target: targetHost, MaximumSeconds: 15, Lanes: make([]laneEvidence, 2)}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range report.Lanes {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			report.Lanes[index] = probeLane(ctx, index+1, deps)
		}(i)
	}
	close(start)
	wg.Wait()
	if report.Lanes[0].TLS && report.Lanes[1].TLS {
		report.Status = "passed"
	}
	return report
}

func main() {
	if len(os.Args) != 1 {
		os.Exit(2)
	}
	report := runProbe(context.Background(), defaultDependencies())
	if json.NewEncoder(os.Stdout).Encode(report) != nil {
		os.Exit(2)
	}
	if report.Status != "passed" {
		os.Exit(1)
	}
}
