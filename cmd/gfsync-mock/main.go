// Command gfsync-mock runs the offline mock of the site's /api/sync/*
// endpoints on 127.0.0.1 for manual testing (MANUAL.md).
//
//	go run ./cmd/gfsync-mock -addr 127.0.0.1:8787
//	GFSync.exe --api http://127.0.0.1:8787
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"gfsync/internal/mockapi"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address (loopback only)")
	autoApprove := flag.Bool("auto-approve", false, "approve pairing codes without clicking")
	fail := flag.Int("fail", 0, "make the first N uploads fail with HTTP 503")
	flag.Parse()

	host, _, err := net.SplitHostPort(*addr)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		log.Fatal("mock server listens on loopback only")
	}
	m := mockapi.New()
	m.Interval = 2
	m.BaseURL = "http://" + *addr
	if *fail > 0 {
		m.FailNext(503, 0, *fail)
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		if *autoApprove && r.URL.Path == "/api/sync/device/start" {
			rec := &capture{ResponseWriter: w}
			m.ServeHTTP(rec, r)
			var s struct{ UserCode string }
			json.Unmarshal(rec.body, &s)
			go func() { time.Sleep(time.Second); m.Approve(s.UserCode) }()
			return
		}
		m.ServeHTTP(w, r)
		if r.URL.Path == "/api/sync/upload" {
			for _, raw := range m.Received() {
				var x struct{ ID, Kind string }
				json.Unmarshal(raw, &x)
				fmt.Printf("  received %s (%s)\n", x.ID, x.Kind)
			}
		}
	})
	log.Printf("GF Sync mock API on http://%s  (auto-approve=%v)", *addr, *autoApprove)
	log.Fatal(http.ListenAndServe(*addr, h))
}

type capture struct {
	http.ResponseWriter
	body []byte
}

func (c *capture) Write(b []byte) (int, error) {
	c.body = append(c.body, b...)
	return c.ResponseWriter.Write(b)
}
