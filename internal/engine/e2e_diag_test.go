package engine_test

import (
	"strings"
	"testing"

	"gfsync/internal/mockapi"
)

// The site (or Cloudflare in front of it) answers an HTML 503 page during
// an outage: the log says what came back once, with the CF-Ray, and
// never anything from the request.
func TestE2EHTML503IsDescribedOnce(t *testing.T) {
	h := newHarness(t)
	h.writeSV("A", sv(2, true, "a:1:character.self"))
	h.mock.FailNextRaw(503, "text/html; charset=UTF-8", mockapi.CloudflareErrorPage, "8f0000000000a1b2-TPE", 3)
	h.start()
	h.waitFor("upload after retries", func() bool { return h.receivedIDs()["a:1:character.self"] })
	h.stop()

	var retries, described []string
	for _, l := range h.logs {
		if strings.HasPrefix(l, "upload: HTTP 503") && strings.Contains(l, "retry in") {
			retries = append(retries, l)
		}
		if strings.Contains(l, "HTTP 503 response:") {
			described = append(described, l)
		}
		if strings.Contains(l, "Bearer") || strings.Contains(l, "Authorization") || strings.Contains(l, `"items"`) {
			t.Errorf("request data in the log: %q", l)
		}
	}
	if len(retries) != 3 {
		t.Fatalf("want 3 retry lines, got %d:\n%s", len(retries), strings.Join(h.logs, "\n"))
	}
	if len(described) != 1 {
		t.Fatalf("want the body described once per hour, got %d:\n%s", len(described), strings.Join(described, "\n"))
	}
	d := described[0]
	for _, want := range []string{"cf-ray=8f0000000000a1b2-TPE", "content-type=text/html; charset=UTF-8",
		"<!DOCTYPE html><html", "503: Service temporarily unavailable"} {
		if !strings.Contains(d, want) {
			t.Errorf("missing %q in %q", want, d)
		}
	}
	if strings.ContainsAny(d, "\r\n") {
		t.Errorf("not one line: %q", d)
	}
}
