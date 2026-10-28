//go:build ignore

package main

import (
	"io"
	"net"
	"os"
	"testing"
	"time"
)

func relay115SocketPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	server, err := listener.Accept()
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); server.Close() })
	return client, server
}

func relay115Fixture(t *testing.T, deadline time.Time) (net.Conn, <-chan struct{}) {
	t.Helper()
	client, inbound := relay115SocketPair(t)
	outbound, echo := relay115SocketPair(t)
	done := make(chan struct{})
	go func() { io.Copy(echo, echo); echo.Close() }()
	go func() { relay115Copy(inbound, outbound, deadline); close(done) }()
	return client, done
}

func relay115RoundTrip(client net.Conn) error {
	client.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Write([]byte("synthetic-round-trip")); err != nil {
		return err
	}
	got := make([]byte, len("synthetic-round-trip"))
	if _, err := io.ReadFull(client, got); err != nil {
		return err
	}
	if string(got) != "synthetic-round-trip" {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func TestLaba115ProxyWholeDeadlineClosesBothPumps(t *testing.T) {
	client, done := relay115Fixture(t, time.Now().Add(100*time.Millisecond))
	if err := relay115RoundTrip(client); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("whole deadline failed to join both relay pumps")
	}
	if err := relay115RoundTrip(client); err == nil {
		t.Fatal("expired relay remained usable")
	}
}

func TestLaba115ProxyReusedConnectionOutlivesFiveMinutes(t *testing.T) {
	if os.Getenv("BUTLER_TEST_LABA_PROXY_LONG_REUSE") != "1" {
		t.Skip("explicit real five-minute zero-auth socket regression")
	}
	started := time.Now()
	client, done := relay115Fixture(t, started.Add(5*time.Minute+15*time.Second))
	turns := 0
	for time.Since(started) < 5*time.Minute+3*time.Second {
		if err := relay115RoundTrip(client); err != nil {
			t.Fatalf("persistent connection failed after %s (%d completed round trips): %v", time.Since(started), turns, err)
		}
		turns++
		time.Sleep(time.Second)
	}
	if err := relay115RoundTrip(client); err != nil {
		t.Fatal("post-five-minute round trip failed", err)
	}
	client.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not join after client close")
	}
	t.Logf("PERSISTENT_SOCKET_PASS elapsed=%s completed_round_trips=%d auth=0 generation=0", time.Since(started), turns+1)
}
