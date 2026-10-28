package main

import (
	"io"
	"net"
	"time"
)

func relay115Copy(client, upstream net.Conn, deadline time.Time) {
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
}
