package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Drives the real _build/_status handlers against a fake build script and
// checks the state transitions the CMS relies on: running → done (with the
// zip path), plus the after-restart fallback that reconstructs state from the
// log alone. Auth is srv's job (jwt_check wraps these in production) and is
// not exercised here.
func TestBuildStatusFlow(t *testing.T) {
	tmpdir = t.TempDir()

	script = tmpdir + "/fake.sh"
	fake := "#!/bin/sh\n" +
		"echo \"Running fetch.sh\"\n" +
		"sleep 1\n" +
		"echo done\n" +
		"echo /departer/builds/energyaccessexplorer-$2.zip\n"
	if err := os.WriteFile(script, []byte(fake), 0755); err != nil {
		t.Fatal(err)
	}

	builds_mu.Lock()
	builds = map[string]*build_rec{}
	builds_mu.Unlock()

	post := httptest.NewRecorder()
	_build(post, httptest.NewRequest("POST", "/build", strings.NewReader(`{"ids":["x"],"os":"linux","depth":0}`)))

	if post.Code != http.StatusOK {
		t.Fatalf("POST /build: %d %s", post.Code, post.Body.String())
	}
	var b struct{ Id string `json:"id"` }
	if err := json.NewDecoder(post.Body).Decode(&b); err != nil || b.Id == "" {
		t.Fatalf("no build id in %q: %v", post.Body.String(), err)
	}

	status := func() (int, map[string]string) {
		rec := httptest.NewRecorder()
		_status(rec, httptest.NewRequest("GET", "/status/"+b.Id, nil))
		var m map[string]string
		_ = json.NewDecoder(rec.Body).Decode(&m)
		return rec.Code, m
	}

	code, m := status()
	if code != http.StatusOK || m["state"] != "running" {
		t.Fatalf("while building: %d %v", code, m)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		code, m = status()
		if m["state"] == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never finished: %d %v", code, m)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if zip := m["zip"]; zip != "/departer/builds/energyaccessexplorer-"+b.Id+".zip" {
		t.Fatalf("zip path: %q", zip)
	}

	// Unknown build → 404; malformed id → 400.
	rec := httptest.NewRecorder()
	_status(rec, httptest.NewRequest("GET", "/status/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	_status(rec, httptest.NewRequest("GET", "/status/", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", rec.Code)
	}

	// After a "restart" (memory empty) the same answer comes from the log.
	builds_mu.Lock()
	builds = map[string]*build_rec{}
	builds_mu.Unlock()
	code, m = status()
	if code != http.StatusOK || m["state"] != "done" || m["zip"] == "" {
		t.Fatalf("after restart: %d %v", code, m)
	}

	fmt.Println("status flow ok:", m)
}
