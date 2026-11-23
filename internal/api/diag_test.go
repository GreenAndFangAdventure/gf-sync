package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errorFrom answers one upload with status, headers and body, and returns
// the client's error.
func errorFrom(t *testing.T, status int, hdr map[string]string, body string) *HTTPError {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range hdr {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "0.2.0")
	_, err := c.Upload(context.Background(), "secret-token-123", "", nil)
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("err = %v, want *HTTPError", err)
	}
	return he
}

func TestDiagHTMLErrorPage(t *testing.T) {
	body := "<html>\r\n<head><title>503 Service Unavailable</title></head>\n\t<body>\x00\x07bad" +
		" echoed secret-token-123 " + strings.Repeat("x", 300)
	he := errorFrom(t, 503, map[string]string{
		"CF-Ray":       "8f0000000000a1b2-TPE",
		"Content-Type": "text/html; charset=UTF-8",
	}, body)
	if he.CFRay != "8f0000000000a1b2-TPE" {
		t.Fatalf("CFRay = %q", he.CFRay)
	}
	d := he.Diag
	for _, want := range []string{
		"cf-ray=8f0000000000a1b2-TPE",
		"content-type=text/html; charset=UTF-8",
		"<html><head><title>503 Service Unavailable</title></head> <body>??bad echoed *** x",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("diag missing %q:\n%s", want, d)
		}
	}
	if strings.ContainsAny(d, "\r\n\t\x00\x07") || strings.Contains(d, "secret-token-123") {
		t.Errorf("diag not one clean line / leaks the token: %q", d)
	}
	// Only the first DiagBodyBytes of the body (after masking the token).
	_, after, _ := strings.Cut(d, "): ")
	if n := len(after); n > DiagBodyBytes || n < DiagBodyBytes-20 {
		t.Errorf("body part is %d bytes, want about %d: %q", n, DiagBodyBytes, after)
	}
	if strings.Contains(he.Error(), "<html>") {
		t.Errorf("Error() should stay short: %q", he.Error())
	}
}

func TestDiagOnlyForUnrecognizedBodies(t *testing.T) {
	he := errorFrom(t, 503, map[string]string{"CF-Ray": "8f0000000000a1b2-TPE", "Content-Type": "application/json"},
		`{"error":"maintenance","message":"back soon"}`)
	if he.Diag != "" || he.Message != "back soon" {
		t.Fatalf("our JSON error: Diag = %q, Message = %q", he.Diag, he.Message)
	}
	he = errorFrom(t, 502, map[string]string{"Content-Type": "application/json"}, `{"ok":false}`)
	if !strings.Contains(he.Diag, `cf-ray=- content-type=application/json`) || !strings.Contains(he.Diag, `{"ok":false}`) {
		t.Fatalf("JSON without error/message should be described: %q", he.Diag)
	}
	he = errorFrom(t, 503, nil, "")
	if !strings.Contains(he.Diag, "cf-ray=-") {
		t.Fatalf("empty body: %q", he.Diag)
	}
}

func TestOneLineCutUTF8(t *testing.T) {
	// "維護中" is 9 bytes; cutting at 7 leaves a broken last rune.
	if got := oneLine([]byte("維護中"), 7); got != "維護?" {
		t.Fatalf("oneLine = %q, want %q", got, "維護?")
	}
}
