package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRequiresHTTPS(t *testing.T) {
	for _, ok := range []string{"https://greenandfangadventure.com", "http://127.0.0.1:8787", "http://localhost:1"} {
		if _, err := New(ok, "1"); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"http://greenandfangadventure.com", "ftp://x", "file:///c:/x"} {
		if _, err := New(bad, "1"); err == nil {
			t.Errorf("%s should be refused", bad)
		}
	}
}

func TestSafeVerifyURL(t *testing.T) {
	c, _ := New(DefaultBase, "1")
	cases := map[string]string{
		"https://greenandfangadventure.com/pages/sync-pair?code=ABCD2345": "https://greenandfangadventure.com/pages/sync-pair?code=ABCD2345",
		"https://evil.example/pages/sync-pair?code=ABCD2345":              "https://greenandfangadventure.com/pages/sync-pair?code=ABCD2345",
		"http://greenandfangadventure.com/pages/sync-pair":                "https://greenandfangadventure.com/pages/sync-pair?code=ABCD2345",
		"":                    "https://greenandfangadventure.com/pages/sync-pair?code=ABCD2345",
		"javascript:alert(1)": "https://greenandfangadventure.com/pages/sync-pair?code=ABCD2345",
		"https://greenandfangadventure.com.evil.example/": "https://greenandfangadventure.com/pages/sync-pair?code=ABCD2345",
	}
	for in, want := range cases {
		if got := c.SafeVerifyURL(&DeviceStart{VerifyURL: in, UserCode: "ABCD2345"}); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	if parseRetryAfter("120") != 120*time.Second || parseRetryAfter("") != 0 || parseRetryAfter("x") != 0 {
		t.Fatal("seconds form")
	}
	future := time.Now().Add(time.Minute).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future); d < 50*time.Second || d > 61*time.Second {
		t.Fatalf("date form: %s", d)
	}
}

func TestPollShapes(t *testing.T) {
	var status int
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "1")
	ctx := context.Background()
	check := func(s int, b string) (*PollResult, error) { status, body = s, b; return c.PollDevice(ctx, "d") }

	if r, err := check(200, `{"pending":true}`); err != nil || !r.Pending {
		t.Error("pending", r, err)
	}
	if r, err := check(200, `{"token":"t"}`); err != nil || r.Token != "t" {
		t.Error("token", r, err)
	}
	if r, err := check(200, `{"error":"slow_down"}`); err != nil || !r.SlowDown {
		t.Error("slow_down", r, err)
	}
	if r, err := check(429, `{}`); err != nil || !r.SlowDown {
		t.Error("429", r, err)
	}
	for _, s := range []int{400, 403, 404, 410} {
		if _, err := check(s, `{"error":"expired_token"}`); !errors.Is(err, ErrPairingEnded) {
			t.Errorf("%d: %v", s, err)
		}
	}
	if _, err := check(200, `{"error":"access_denied"}`); !errors.Is(err, ErrPairingEnded) {
		t.Error("denied", err)
	}
	if _, err := check(200, `{}`); err == nil {
		t.Error("empty body must error")
	}
	if _, err := check(500, `oops`); err == nil || errors.Is(err, ErrPairingEnded) {
		t.Error("500 is transient", err)
	}
	if _, err := check(200, `not json`); err == nil {
		t.Error("bad json")
	}
}

func TestUploadUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"results":[{"id":"a","status":"accepted"}]}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "1")
	if _, err := c.Upload(context.Background(), "bad", "", nil); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	r, err := c.Upload(context.Background(), "good", "", nil)
	if err != nil || len(r) != 1 || r[0].Status != "accepted" {
		t.Fatal(r, err)
	}
}

func TestStartDeviceBody(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"deviceCode":"d","userCode":"ABCD2345","verifyUrl":"","interval":3,"expiresIn":600}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "0.1.0")
	c.Label = "MY-PC"
	if _, err := c.StartDevice(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["label"] != "MY-PC" || got["clientVersion"] != "0.1.0" {
		t.Fatalf("start body %v", got)
	}
	c.Label = ""
	c.StartDevice(context.Background())
	if got["label"] != "GF Sync" {
		t.Fatalf("default label %v", got)
	}
}

func TestPollTokenWithDeviceIDAndSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/sync/upload" {
			w.Write([]byte(`{"results":[{"id":"a","status":"accepted","skipped":3}]}`))
			return
		}
		w.Write([]byte(`{"token":"t","deviceId":42}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "1")
	r, err := c.PollDevice(context.Background(), "d")
	if err != nil || r.Token != "t" || r.Pending {
		t.Fatal(r, err)
	}
	res, err := c.Upload(context.Background(), "t", "", nil)
	if err != nil || res[0].Status != "accepted" {
		t.Fatal(res, err)
	}
}

func TestPollDeniedVersusExpired(t *testing.T) {
	cases := []struct {
		status int
		body   string
		denied bool
	}{
		{403, `{"error":"denied"}`, true},
		{200, `{"error":"access_denied"}`, true},
		{410, `{"error":"expired_token"}`, false},
		{200, `{"error":"expired_token"}`, false},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(c.status)
			w.Write([]byte(c.body))
		}))
		cl, err := New(srv.URL, "t")
		if err != nil {
			t.Fatal(err)
		}
		_, err = cl.PollDevice(context.Background(), "d")
		srv.Close()
		if !errors.Is(err, ErrPairingEnded) || errors.Is(err, ErrPairingDenied) != c.denied {
			t.Errorf("%d %s: err %v", c.status, c.body, err)
		}
	}
}

func TestUploadRequestAddonVersion(t *testing.T) {
	c, err := New("https://example.com", "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c.NewUploadRequest("0.10.0", nil))
	if !strings.Contains(string(b), `"addonVersion":"0.10.0"`) {
		t.Errorf("%s", b)
	}
	b, _ = json.Marshal(c.NewUploadRequest("", nil))
	if strings.Contains(string(b), "addonVersion") {
		t.Errorf("an unknown AddOn version is left out: %s", b)
	}
}
