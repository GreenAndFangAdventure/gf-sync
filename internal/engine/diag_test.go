package engine

import (
	"testing"
	"time"

	"gfsync/internal/api"
)

func TestDiagLimiterOncePerHour(t *testing.T) {
	var d diagLimiter
	t0 := time.Date(2026, 11, 23, 1, 26, 0, 0, time.UTC)
	k := diagKey(&api.HTTPError{Status: 503, CFRay: "8f0000000000a1b2-TPE"})
	if !d.allow(k, t0) {
		t.Fatal("first one must be logged")
	}
	// A retry minutes later has a new ray ID from the same data center.
	k2 := diagKey(&api.HTTPError{Status: 503, CFRay: "8f0000000000c3d4-TPE"})
	if k2 != k || d.allow(k2, t0.Add(10*time.Minute)) {
		t.Fatalf("same status and data center within the hour: key %q vs %q", k2, k)
	}
	if !d.allow(diagKey(&api.HTTPError{Status: 502, CFRay: "8f0000000000c3d4-TPE"}), t0.Add(time.Minute)) {
		t.Fatal("another status is another kind")
	}
	if !d.allow(diagKey(&api.HTTPError{Status: 503, CFRay: "8f0000000000c3d4-NRT"}), t0.Add(time.Minute)) {
		t.Fatal("another data center is another kind")
	}
	if !d.allow(k, t0.Add(DiagEvery)) {
		t.Fatal("logged again after an hour")
	}
	if diagKey(&api.HTTPError{Status: 503}) != "503 -" || diagKey(&api.HTTPError{Status: 503, CFRay: "odd"}) != "503 ?" {
		t.Fatal("keys without a usable CF-Ray")
	}
}
