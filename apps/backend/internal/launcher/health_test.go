package launcher

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type neverExitedProcess struct{}

func (neverExitedProcess) Exited() (bool, int) {
	return false, 0
}

func TestWaitForHealthDoesNotStallPastDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	started := time.Now()
	err := waitForHealth(server.URL, neverExitedProcess{}, 100*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected health check to time out")
	}
	if elapsed := time.Since(started); elapsed > 450*time.Millisecond {
		t.Fatalf("health check stalled for %s", elapsed)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestWaitForURLDoesNotStallPastDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	started := time.Now()
	err := waitForURL(server.URL, neverExitedProcess{}, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected URL check to time out")
	}
	if elapsed := time.Since(started); elapsed > 450*time.Millisecond {
		t.Fatalf("URL check stalled for %s", elapsed)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timeout", err)
	}
}
