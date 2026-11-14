package sink

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestLocalStatusReadsARunningInstance(t *testing.T) {
	want := Status{
		Station: "homedesktop streaming audio",
		Format:  "wav",
		Bitrate: 1536,
		URI:     "http://192.168.0.29:8567/stream.wav",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer server.Close()

	got, ok := LocalStatus(portOf(t, server.URL))
	if !ok {
		t.Fatal("LocalStatus did not recognise the instance")
	}
	if got.Station != want.Station || got.URI != want.URI || got.Bitrate != want.Bitrate {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Another service can answer 200 with valid JSON on /status in an unrelated
// process, so a parseable body is not enough to call it an instance.
func TestLocalStatusRejectsForeignJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	if _, ok := LocalStatus(portOf(t, server.URL)); ok {
		t.Error("a foreign JSON body was accepted as an instance")
	}
}

func TestLocalStatusRejectsAnErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, ok := LocalStatus(portOf(t, server.URL)); ok {
		t.Error("a 500 was accepted as an instance")
	}
}

func TestLocalStatusRejectsAClosedPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	port := portOf(t, server.URL)
	server.Close()

	if _, ok := LocalStatus(port); ok {
		t.Error("a closed port was accepted as an instance")
	}
}

func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}
