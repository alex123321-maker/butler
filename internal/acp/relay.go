package acp

import (
	"context"
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/butler/butler/internal/security/egress"
)

func ServeGoogleEgress(ctx context.Context, listener net.Listener) error {
	return ServeGoogleEgressVia(ctx, listener, "")
}

func ServeGoogleEgressVia(ctx context.Context, listener net.Listener, upstream string) error {
	dial, err := googleUpstreamDial(upstream)
	if err != nil {
		return err
	}
	policy := egress.Policy{AllowedDomains: []string{"accounts.google.com", "oauth2.googleapis.com", "daily-cloudcode-pa.googleapis.com", "cloudcode-pa.googleapis.com", "aicode.googleapis.com", "www.googleapis.com", "antigravity.google", "lh3.googleusercontent.com"}, Resolver: newGoogleDNSResolver()}
	policy.Dial = dial
	if dial != nil {
		policy.Resolver = newGoogleDNSResolverVia(dial)
	}
	return serveRelay(ctx, listener, func() (net.Conn, func(), error) {
		proxy, err := egress.NewBrowserProxy(policy, "https://accounts.google.com", time.Now().Add(305*time.Second), 8<<20)
		if err != nil {
			return nil, nil, ErrProcess
		}
		address, _ := url.Parse(proxy.URL())
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", address.Host)
		return conn, func() { _ = proxy.Close() }, err
	})
}

func ServeLocalProxy(ctx context.Context, listener net.Listener, socket string) error {
	return serveRelay(ctx, listener, func() (net.Conn, func(), error) {
		conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", socket)
		return conn, func() {}, err
	})
}

func serveRelay(ctx context.Context, listener net.Listener, dial func() (net.Conn, func(), error)) error {
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	slots := make(chan struct{}, 16)
	for {
		client, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return ErrProcess
		}
		select {
		case slots <- struct{}{}:
			go func() {
				defer func() { <-slots }()
				defer client.Close()
				remote, cleanup, err := dial()
				if cleanup != nil {
					defer cleanup()
				}
				if err != nil {
					return
				}
				defer remote.Close()
				deadline := time.Now().Add(305 * time.Second)
				_ = client.SetDeadline(deadline)
				_ = remote.SetDeadline(deadline)
				closeBoth := func() { _ = client.Close(); _ = remote.Close() }
				stop := context.AfterFunc(ctx, closeBoth)
				defer stop()
				var pumps sync.WaitGroup
				pumps.Add(2)
				go func() { defer pumps.Done(); _, _ = io.Copy(remote, client); closeBoth() }()
				go func() { defer pumps.Done(); _, _ = io.Copy(client, remote); closeBoth() }()
				pumps.Wait()
			}()
		default:
			_ = client.Close()
		}
	}
}
