package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

func connectionClass(err error) string {
	if err == nil {
		return "NONE"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "DNS"
	}
	var ca x509.UnknownAuthorityError
	if errors.As(err, &ca) {
		return "TLS_CA"
	}
	var host x509.HostnameError
	if errors.As(err, &host) {
		return "TLS_HOSTNAME"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "TIMEOUT"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "CONNECTION_REFUSED"
	}
	if errors.Is(err, syscall.ECONNRESET) {
		return "CONNECTION_RESET"
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "EOF"
	}
	if strings.Contains(err.Error(), "407") {
		return "PROXY_AUTH"
	}
	if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "Forbidden") {
		return "PROXY_DENIED"
	}
	return "UNKNOWN"
}

func probeNetwork() {
	if loadOperatorEnvironment() != nil {
		safeExit("PRIVATE_ENVIRONMENT_FAILED")
	}
	proxy, err := url.Parse(os.Getenv("HTTPS_PROXY"))
	if err != nil || proxy.Scheme != "http" {
		safeExit("PROXY_KIND_UNSUPPORTED")
	}
	for _, host := range []string{"oauth2.googleapis.com", "aicode.googleapis.com"} {
		ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
		p := map[string]any{"host": host, "phase": "CONNECT_TLS_ONLY", "real_model_calls": 0, "oauth_refresh_calls": 0, "proxy_connect_http_status": 0, "upstream_tls_ready": false}
		address := proxy.Host
		if proxy.Port() == "" {
			address = net.JoinHostPort(proxy.Hostname(), "80")
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
		if err != nil {
			p["failure_enum"] = connectionClass(err)
			_ = json.NewEncoder(os.Stdout).Encode(p)
			stop()
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(18 * time.Second))
		req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: host + ":443"}, Host: host + ":443", Header: http.Header{}}
		if proxy.User != nil {
			password, _ := proxy.User.Password()
			req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxy.User.Username()+":"+password)))
		}
		err = req.Write(conn)
		if err == nil {
			var resp *http.Response
			resp, err = http.ReadResponse(bufio.NewReader(conn), req)
			if err == nil {
				p["proxy_connect_http_status"] = resp.StatusCode
				if resp.StatusCode == 200 {
					secure := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
					err = secure.HandshakeContext(ctx)
					p["upstream_tls_ready"] = err == nil
					_ = secure.Close()
				}
			}
		}
		p["failure_enum"] = connectionClass(err)
		_ = conn.Close()
		stop()
		_ = json.NewEncoder(os.Stdout).Encode(p)
		requestPath := "/.well-known/openid-configuration"
		if host == "aicode.googleapis.com" {
			requestPath = "/v1:fetchConfig"
		}
		reqHTTP, _ := http.NewRequest("GET", "https://"+host+requestPath, nil)
		tr := http.DefaultTransport.(*http.Transport).Clone()
		proxySelected, _ := tr.Proxy(reqHTTP)
		q := map[string]any{"phase": "GO_HTTP_GET_WITHOUT_AUTH", "host": host, "real_model_calls": 0, "oauth_refresh_calls": 0, "proxy_selected": proxySelected != nil, "http_status": 0}
		response, requestError := (&http.Client{Transport: tr, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(reqHTTP)
		if response != nil {
			q["http_status"] = response.StatusCode
			_ = response.Body.Close()
		}
		q["failure_enum"] = connectionClass(requestError)
		_ = json.NewEncoder(os.Stdout).Encode(q)
	}
}
