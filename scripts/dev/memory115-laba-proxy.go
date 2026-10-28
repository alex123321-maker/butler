//go:build ignore

// Fixed byte relay to the already running laba proxy. No TLS termination,
// headers, destination selection, authentication or payload logging occurs here.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const bridge115Proxy = "10.249.116.1:18081"

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	if os.Args[1] == "probe" {
		proxy, _ := url.Parse("http://" + bridge115Proxy)
		transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
		defer transport.CloseIdleConnections()
		client := http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, _ := http.NewRequest(http.MethodHead, "https://daily-cloudcode-pa.googleapis.com/", nil)
		response, err := client.Do(req)
		if err != nil {
			fmt.Println("FAIL fixed existing-proxy TLS route; auth=0 generation=0")
			os.Exit(1)
		}
		response.Body.Close()
		fmt.Printf("PASS fixed existing-proxy TLS route HTTP=%d auth=0 generation=0\n", response.StatusCode)
		return
	}
	if os.Args[1] != "serve" {
		os.Exit(2)
	}
	listener, err := net.Listen("tcp4", bridge115Proxy)
	if err != nil {
		fmt.Println("FAIL fixed bridge listener")
		os.Exit(1)
	}
	defer listener.Close()
	deadline := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	go func() { <-ctx.Done(); listener.Close() }()
	limit := make(chan struct{}, 8)
	var joined sync.WaitGroup
	var admitted, denied atomic.Uint64
	fmt.Println("READY fixed bridge-to-existing-loopback-proxy; payload_logging=false")
	for {
		client, acceptErr := listener.Accept()
		if acceptErr != nil {
			break
		}
		ip, _, _ := net.SplitHostPort(client.RemoteAddr().String())
		if ip != "10.249.116.2" {
			denied.Add(1)
			client.Close()
			continue
		}
		select {
		case limit <- struct{}{}:
		default:
			denied.Add(1)
			client.Close()
			continue
		}
		joined.Add(1)
		go func() {
			defer joined.Done()
			defer func() { <-limit }()
			defer client.Close()
			upstream, dialErr := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp4", "127.0.0.1:18080")
			if dialErr != nil {
				return
			}
			defer upstream.Close()
			admitted.Add(1)
			end := time.Now().Add(5 * time.Minute)
			if deadline.Before(end) {
				end = deadline
			}
			client.SetDeadline(end)
			upstream.SetDeadline(end)
			done := make(chan struct{})
			go func() {
				io.Copy(upstream, client)
				if tcp, ok := upstream.(*net.TCPConn); ok {
					tcp.CloseWrite()
				}
				close(done)
			}()
			io.Copy(client, upstream)
			client.Close()
			upstream.Close()
			<-done
		}()
	}
	joined.Wait()
	fmt.Printf("EXIT admitted_connections=%d denied_connections=%d payload_logging=false\n", admitted.Load(), denied.Load())
}
