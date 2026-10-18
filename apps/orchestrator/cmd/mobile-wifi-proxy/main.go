// mobile-wifi-proxy exposes only the phone ingress over an isolated Wi-Fi test
// route. Its upstream is a loopback SSH forward, never a model-selected target.
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

func main() {
	listen := flag.String("listen", ":18061", "Wi-Fi acceptance ingress")
	upstream := flag.String("upstream", "http://127.0.0.1:18571", "loopback SSH forwarding endpoint")
	flag.Parse()
	target, err := url.Parse(*upstream)
	if err != nil || target.Scheme != "http" || target.User != nil || target.Path != "" || target.RawQuery != "" || net.ParseIP(target.Hostname()) == nil || !net.ParseIP(target.Hostname()).IsLoopback() {
		log.Fatal("loopback test upstream required")
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "test upstream unavailable", 502)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/mobile/") || r.URL.RawPath != "" || r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
		proxy.ServeHTTP(w, r)
	})
	log.Print("isolated Wi-Fi phone ingress ready")
	log.Fatal((&http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe())
}
