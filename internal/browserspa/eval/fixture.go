// Package eval is an independent operator-owned SPA recipient. It has no tool,
// authorization, proxy, secret-store or production network-policy exception.
package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	Origin   string `json:"origin"`
	Variant  string `json:"variant"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Marker   string `json:"marker"`
	Stock    int    `json:"stock"`
	Scenario string `json:"scenario,omitempty"`
}

type Event struct {
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Category string    `json:"category,omitempty"`
	Bytes    int       `json:"bytes,omitempty"`
	Accepted bool      `json:"accepted"`
}

type Fixture struct {
	config Config
	record func(Event)
	prefix string
	worker atomic.Int64
}

func New(c Config, record func(Event)) (*Fixture, error) {
	u, err := url.Parse(c.Origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || record == nil || len(c.Variant) < 8 || len(c.Variant) > 32 || len(c.Category) == 0 || len(c.Category) > 64 || len(c.Title) == 0 || len(c.Title) > 128 || len(c.Marker) < 8 || len(c.Marker) > 64 || c.Stock < 1 || c.Stock > 9999 {
		return nil, fmt.Errorf("invalid isolated SPA fixture")
	}
	for _, char := range c.Variant {
		if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') {
			return nil, fmt.Errorf("invalid fixture variant")
		}
	}
	switch c.Scenario {
	case "", "normal", "mutation", "private", "foreign", "storm", "oversized", "infinite", "lost_query", "ws_send", "worker_private", "worker_mutation":
	default:
		return nil, fmt.Errorf("invalid fixture scenario")
	}
	return &Fixture{config: c, record: record, prefix: "/spa-" + c.Variant + "/"}, nil
}

func (f *Fixture) URL() string { return f.config.Origin + f.prefix }
func (f *Fixture) event(kind string, accepted bool, category string, bytes int) {
	f.record(Event{At: time.Now().UTC(), Kind: kind, Accepted: accepted, Category: category, Bytes: bytes})
}
func (f *Fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !strings.HasPrefix(r.URL.Path, f.prefix) {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, f.prefix)
	switch path {
	case "":
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		f.event("document", true, "", 0)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		public, _ := json.Marshal(map[string]string{"prefix": f.prefix, "origin": f.config.Origin, "scenario": f.config.Scenario})
		_, _ = fmt.Fprintf(w, page, string(public))
	case "worker.js":
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		f.worker.Add(1)
		f.event("worker_script", true, "", 0)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Service-Worker-Allowed", f.prefix)
		public, _ := json.Marshal(map[string]string{"prefix": f.prefix, "scenario": f.config.Scenario})
		_, _ = fmt.Fprintf(w, worker, string(public))
	case "cached":
		f.event("cache_origin", true, "", 0)
		_, _ = io.WriteString(w, "cache-"+f.config.Marker)
	case "graphql":
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var body struct {
			Query     string `json:"query"`
			Variables struct {
				Category string `json:"category"`
			} `json:"variables"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
		if dec.Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		// This recipient does not import or trust the runtime query validator.
		accepted := body.Query == `query Catalog($category:String!){catalog(category:$category){title marker category}}`
		f.event("query", accepted, body.Variables.Category, 0)
		if !accepted {
			w.WriteHeader(422)
			return
		}
		if f.config.Scenario == "lost_query" {
			c, _, e := w.(http.Hijacker).Hijack()
			if e == nil {
				c.Close()
			}
			return
		}
		category, title := "all", "General catalog"
		if body.Variables.Category == f.config.Category {
			category, title = f.config.Category, f.config.Title
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"catalog": map[string]string{"title": title, "marker": f.config.Marker, "category": category}}})
	case "events":
		f.event("sse_open", true, "", 0)
		defer f.event("sse_closed", true, "", 0)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "retry: 100\ndata: feed-%s\n\n", f.config.Marker)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	case "stream":
		f.event("stream", true, "", 0)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "stream-"+f.config.Marker)
		w.(http.Flusher).Flush()
		if f.config.Scenario == "infinite" {
			<-r.Context().Done()
			return
		}
	case "feed":
		upgrade := websocket.Upgrader{Subprotocols: []string{"graphql-transport-ws"}, CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == f.config.Origin }}
		conn, e := upgrade.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer conn.Close()
		defer f.event("ws_closed", true, "", 0)
		f.event("ws_open", true, "", 0)
		_ = conn.SetReadDeadline(time.Now().Add(25 * time.Second))
		_, init, e := conn.ReadMessage()
		if e != nil {
			return
		}
		var hello struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(init, &hello) != nil || hello.Type != "connection_init" {
			f.event("ws_client_message", true, "", len(init))
			return
		}
		f.event("ws_init", true, "", len(init))
		_ = conn.WriteJSON(map[string]string{"type": "connection_ack"})
		_, sub, e := conn.ReadMessage()
		if e != nil {
			return
		}
		var subscribe struct {
			Type    string `json:"type"`
			ID      string `json:"id"`
			Payload struct {
				Query string `json:"query"`
			} `json:"payload"`
		}
		if json.Unmarshal(sub, &subscribe) != nil || subscribe.Type != "subscribe" || subscribe.ID != "stock" || subscribe.Payload.Query != "subscription Stock{stock}" {
			f.event("ws_client_message", true, "", len(sub))
			return
		}
		f.event("ws_subscribe", true, "", len(sub))
		data := fmt.Sprintf("stock:%d", f.config.Stock)
		if f.config.Scenario == "oversized" {
			data = strings.Repeat("x", 128*1024)
		}
		_ = conn.WriteJSON(map[string]any{"type": "next", "id": "stock", "payload": map[string]any{"data": map[string]string{"stock": data}}})
		for {
			_, data, e := conn.ReadMessage()
			if e != nil {
				return
			}
			f.event("ws_client_message", true, "", len(data))
		}
	case "effect":
		f.event("forbidden_effect", true, "", 0)
		_, _ = io.WriteString(w, "effect accepted")
	default:
		http.NotFound(w, r)
	}
}

const page = `<!doctype html><html><head><meta charset="utf-8"><title>Live catalog</title></head><body><h1>Live catalog</h1><form><label>Category <input name="category" type="search" autocomplete="off"></label></form><p id="result">Loading interactive data</p><p id="connection"></p><script>
const c=%s;
const state={title:'',marker:'',category:'',stock:'',feed:'',stream:'',worker:false};
function render(){document.getElementById('result').textContent=state.title+' | category: '+state.category+' | receipt: '+state.marker+' | '+state.stock+' | '+state.feed+' | '+state.stream+' | worker: '+state.worker;document.getElementById('result').dataset.ready=String(Boolean(state.title&&state.stock&&state.feed&&state.stream&&state.worker));}
async function query(category){let query='query Catalog($category:String!){catalog(category:$category){title marker category}}';if(c.scenario==='mutation')query='mutation{buy}';const r=await fetch(c.prefix+'graphql',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({query,variables:{category}})});const v=(await r.json()).data.catalog;Object.assign(state,v);render();}
document.querySelector('form').addEventListener('submit',e=>e.preventDefault());
document.querySelector('input').addEventListener('input',e=>query(e.target.value).catch(()=>{}));
(async()=>{const reg=await navigator.serviceWorker.register(c.prefix+'worker.js',{scope:c.prefix,updateViaCache:'none'});await navigator.serviceWorker.ready;if(!navigator.serviceWorker.controller)await new Promise(resolve=>navigator.serviceWorker.addEventListener('controllerchange',resolve,{once:true}));await reg.update();const a=await(await fetch(c.prefix+'cached')).text();const b=await(await fetch(c.prefix+'cached')).text();state.worker=a===b&&a.startsWith('cache-');await query('all');const ws=new WebSocket(c.origin.replace('https:','wss:')+c.prefix+'feed','graphql-transport-ws');ws.onmessage=e=>{const v=JSON.parse(e.data);if(v.type==='connection_ack')ws.send(JSON.stringify({id:'stock',type:'subscribe',payload:{query:'subscription Stock{stock}'}}));if(v.type==='next'){state.stock=v.payload.data.stock;render()}};ws.onopen=()=>{if(c.scenario==='ws_send')ws.send('buy');else ws.send(JSON.stringify({type:'connection_init'}))};const es=new EventSource(c.prefix+'events');es.onmessage=e=>{state.feed=e.data;render()};state.stream=await(await fetch(c.prefix+'stream')).text();render();if(c.scenario==='private')fetch('https://127.0.0.1/effect').catch(()=>{});if(c.scenario==='foreign')fetch('https://foreign.test/effect').catch(()=>{});if(c.scenario==='storm')for(let i=0;i<8;i++)new WebSocket(c.origin.replace('https:','wss:')+c.prefix+'feed','graphql-transport-ws');})().catch(()=>{document.getElementById('connection').textContent='Interactive data unavailable'});
</script></body></html>`

const worker = `const c=%s;
const boot=crypto.randomUUID();self.addEventListener('message',e=>{if(e.data==='probe'&&e.ports[0])e.ports[0].postMessage(boot)});
self.addEventListener('install',e=>e.waitUntil(self.skipWaiting()));
self.addEventListener('activate',e=>e.waitUntil(self.clients.claim()));
self.addEventListener('fetch',e=>{const u=new URL(e.request.url);if(u.pathname===c.prefix+'cached'){e.respondWith((async()=>{const cache=await caches.open('catalog');const hit=await cache.match(e.request);if(hit)return hit;const r=await fetch(e.request);await cache.put(e.request,r.clone());return r;})());}else if(c.scenario==='worker_private'){e.respondWith(fetch('https://127.0.0.1/effect'));}else if(c.scenario==='worker_mutation'){e.respondWith(fetch(c.prefix+'effect',{method:'POST',body:'buy'}));}else{e.respondWith(fetch(e.request));}});
`
