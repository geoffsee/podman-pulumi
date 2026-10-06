package transport

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/geoffsee/pulumi-podman/internal/specadapt"
)

type recorded struct {
	Method      string
	Path        string
	Query       string
	Body        string
	ContentType string
}

type fakeEngine struct {
	mu      sync.Mutex
	hits    []recorded
	handler http.HandlerFunc
}

func (f *fakeEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	f.mu.Lock()
	f.hits = append(f.hits, recorded{
		Method:      r.Method,
		Path:        r.URL.Path,
		Query:       r.URL.RawQuery,
		Body:        string(raw),
		ContentType: r.Header.Get("Content-Type"),
	})
	f.mu.Unlock()
	if f.handler != nil {
		r.Body = io.NopCloser(strings.NewReader(string(raw)))
		f.handler(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeEngine) recorded() []recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recorded, len(f.hits))
	copy(out, f.hits)
	return out
}

func serve(t *testing.T, network string, h http.Handler) Conn {
	t.Helper()
	var ln net.Listener
	var addr string
	var err error
	if network == "unix" {
		dir, mkerr := os.MkdirTemp("/tmp", "prt-*")
		if mkerr != nil {
			t.Fatal(mkerr)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		addr = filepath.Join(dir, "s")
		ln, err = net.Listen("unix", addr)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			addr = ln.Addr().String()
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	return Conn{Network: network, Address: addr}
}

func newClient(t *testing.T, conn Conn, rules []specadapt.Rule) *http.Client {
	t.Helper()
	t.Setenv("PODMAN_API_VERSION", "")
	return New(conn, rules)
}

func do(t *testing.T, client *http.Client, method, path string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, "http://d"+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestRoundTripUnixVolumeInspect(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v6.0.0/libpod/volumes/data/json" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Name":"data"}`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "GET",
		AdaptedPath:    "/volumes/{id}",
		UpstreamMethod: "GET",
		UpstreamPath:   "/v6.0.0/libpod/volumes/{name}/json",
	}})
	resp := do(t, client, http.MethodGet, "/volumes/data", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	if string(raw) != `{"Name":"data"}` {
		t.Fatalf("body = %s", raw)
	}
}

func TestRoundTripTCP(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}}
	client := newClient(t, serve(t, "tcp", eng), []specadapt.Rule{{
		Method:         "GET",
		AdaptedPath:    "/volumes/{id}",
		UpstreamMethod: "GET",
		UpstreamPath:   "/v6.0.0/libpod/volumes/{name}/json",
	}})
	resp := do(t, client, http.MethodGet, "/volumes/n", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	hits := eng.recorded()
	if len(hits) != 1 || hits[0].Path != "/v6.0.0/libpod/volumes/n/json" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestRoundTripStartAfterCreate(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v6.0.0/libpod/containers/create":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Id":"abc123"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v6.0.0/libpod/containers/abc123/start":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/containers",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/containers/create",
		StartPath:      "/v6.0.0/libpod/containers/{id}/start",
	}})
	resp := do(t, client, http.MethodPost, "/containers", strings.NewReader(`{"image":"pause"}`))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	if string(raw) != `{"Id":"abc123"}` {
		t.Fatalf("caller should still see create body, got %s", raw)
	}
	hits := eng.recorded()
	if len(hits) != 2 {
		t.Fatalf("hits = %+v", hits)
	}
	if hits[0].Path != "/v6.0.0/libpod/containers/create" || hits[1].Path != "/v6.0.0/libpod/containers/abc123/start" {
		t.Fatalf("paths = %+v", hits)
	}
}

func TestRoundTripStartAfterCreateUsesName(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v6.0.0/libpod/pods/create":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Name":"mypod"}`))
		case "/v6.0.0/libpod/pods/mypod/start":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, r.URL.Path, 404)
		}
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/pods",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/pods/create",
		StartPath:      "/v6.0.0/libpod/pods/{name}/start",
	}})
	_ = do(t, client, http.MethodPost, "/pods", strings.NewReader(`{"name":"mypod"}`))
	hits := eng.recorded()
	if len(hits) != 2 || hits[1].Path != "/v6.0.0/libpod/pods/mypod/start" {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestRoundTripStartFailure(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/create") {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"Id":"dead"}`))
			return
		}
		http.Error(w, "no such image", http.StatusInternalServerError)
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/containers",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/containers/create",
		StartPath:      "/v6.0.0/libpod/containers/{id}/start",
	}})
	req, _ := http.NewRequest(http.MethodPost, "http://d/containers", strings.NewReader(`{}`))
	_, err := client.Do(req)
	if err == nil || !strings.Contains(err.Error(), "start resource") {
		t.Fatalf("err = %v", err)
	}
}

func TestRoundTripSkipsStartWithoutID(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/containers",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/containers/create",
		StartPath:      "/v6.0.0/libpod/containers/{id}/start",
	}})
	_ = do(t, client, http.MethodPost, "/containers", strings.NewReader(`{}`))
	if n := len(eng.recorded()); n != 1 {
		t.Fatalf("expected create only, hits=%d", n)
	}
}

func TestRoundTripCoercesDeleteArray(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"Ok":true,"Id":"abc"}]`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "DELETE",
		AdaptedPath:    "/containers/{id}",
		UpstreamMethod: "DELETE",
		UpstreamPath:   "/v6.0.0/libpod/containers/{name}",
		DefaultQuery:   map[string]string{"force": "true"},
	}})
	resp := do(t, client, http.MethodDelete, "/containers/abc", nil)
	raw, _ := io.ReadAll(resp.Body)
	if string(raw) != "{}" {
		t.Fatalf("coerced body = %q", raw)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	hits := eng.recorded()
	if hits[0].Query != "force=true" {
		t.Fatalf("query = %s", hits[0].Query)
	}
}

func TestRoundTripImagePullQuery(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sha256:abc"}`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/images",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/images/pull",
		QueryFromBody:  []string{"reference", "quiet"},
		DefaultQuery:   map[string]string{"quiet": "true"},
		BodyMode:       "query",
	}})
	resp := do(t, client, http.MethodPost, "/images", strings.NewReader(`{"reference":"busybox:latest"}`))
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	hits := eng.recorded()
	if len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	if hits[0].Path != "/v6.0.0/libpod/images/pull" {
		t.Fatalf("path = %s", hits[0].Path)
	}
	if hits[0].Body != "" {
		t.Fatalf("body should be empty, got %q", hits[0].Body)
	}
	q := hits[0].Query
	if !strings.Contains(q, "reference=busybox") || !strings.Contains(q, "quiet=true") {
		t.Fatalf("query = %s", q)
	}
}

func TestRoundTripSecretRawBody(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ID":"s1"}`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/secrets",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/secrets/create",
		QueryFromBody:  []string{"name"},
		BodyMode:       "rawString",
		RawStringField: "data",
	}})
	_ = do(t, client, http.MethodPost, "/secrets", strings.NewReader(`{"name":"db","data":"s3cret"}`))
	hits := eng.recorded()
	if hits[0].Body != "s3cret" {
		t.Fatalf("body = %q", hits[0].Body)
	}
	if !strings.Contains(hits[0].Query, "name=db") {
		t.Fatalf("query = %s", hits[0].Query)
	}
	if hits[0].ContentType != "application/octet-stream" {
		t.Fatalf("content-type = %s", hits[0].ContentType)
	}
}

func TestRoundTripNetworkPatch(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "PATCH",
		AdaptedPath:    "/networks/{id}",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/networks/{name}/update",
	}})
	_ = do(t, client, http.MethodPatch, "/networks/n1", strings.NewReader(`{"adddnsservers":["1.1.1.1"]}`))
	hits := eng.recorded()
	if hits[0].Method != http.MethodPost || hits[0].Path != "/v6.0.0/libpod/networks/n1/update" {
		t.Fatalf("hit = %+v", hits[0])
	}
	if hits[0].Body != `{"adddnsservers":["1.1.1.1"]}` {
		t.Fatalf("body = %s", hits[0].Body)
	}
}

func TestRoundTripUnmatchedPathPassthrough(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`OK`))
	}}
	client := newClient(t, serve(t, "unix", eng), nil)
	_ = do(t, client, http.MethodGet, "/v6.0.0/libpod/_ping", nil)
	hits := eng.recorded()
	if hits[0].Path != "/v6.0.0/libpod/_ping" {
		t.Fatalf("path = %s", hits[0].Path)
	}
}

func TestNewAppliesPODMAN_API_VERSION(t *testing.T) {
	t.Setenv("PODMAN_API_VERSION", "5.0.0")
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}}
	client := New(serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/volumes",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/volumes/create",
	}})
	_ = do(t, client, http.MethodPost, "/volumes", strings.NewReader(`{"Name":"x"}`))
	hits := eng.recorded()
	if hits[0].Path != "/v5.0.0/libpod/volumes/create" {
		t.Fatalf("path = %s", hits[0].Path)
	}
}

func TestRewriteAPIPrefixEmptyAndNonLibpod(t *testing.T) {
	got := RewriteAPIPrefix([]specadapt.Rule{
		{UpstreamPath: "", StartPath: ""},
		{UpstreamPath: "/not-libpod"},
	}, "6.0.0")
	if got[0].UpstreamPath != "" || got[0].StartPath != "" {
		t.Fatalf("empty paths rewritten: %+v", got[0])
	}
	if got[1].UpstreamPath != "/not-libpod" {
		t.Fatalf("non-libpod = %s", got[1].UpstreamPath)
	}
}

func TestCoerceJSONObjectWhitespaceArray(t *testing.T) {
	if got := string(coerceJSONObject([]byte("  \n[1]"))); got != "{}" {
		t.Fatalf("got %q", got)
	}
}

func TestPathRegexpAllowsSlashes(t *testing.T) {
	t.Parallel()
	re := pathRegexp("/images/{id}")
	m := re.FindStringSubmatch("/images/localhost/busybox:latest")
	if m == nil {
		t.Fatal("expected match")
	}
	idx := re.SubexpIndex("id")
	if idx < 0 || m[idx] != "localhost/busybox:latest" {
		t.Fatalf("id = %v", m)
	}
	if re.MatchString("/images") {
		t.Fatal("collection path should not match item template")
	}
}

func TestPathRegexpUnclosedBrace(t *testing.T) {
	t.Parallel()
	re := pathRegexp("/images/{id")
	if re.MatchString("/images/foo") {
		t.Fatal("unclosed brace should not produce a working matcher")
	}
}

func TestRewriteAPIPrefixLibpodExact(t *testing.T) {
	got := RewriteAPIPrefix([]specadapt.Rule{{UpstreamPath: "/libpod"}}, "6.0.0")
	if got[0].UpstreamPath != "/v6.0.0/libpod" {
		t.Fatalf("got %s", got[0].UpstreamPath)
	}
}

func TestRoundTripSlashImageID(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v6.0.0/libpod/images/localhost/busybox:latest/json" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Id":"abc"}`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "GET",
		AdaptedPath:    "/images/{id}",
		UpstreamMethod: "GET",
		UpstreamPath:   "/v6.0.0/libpod/images/{name}/json",
	}})
	resp := do(t, client, http.MethodGet, "/images/localhost/busybox:latest", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestRoundTripDoesNotStartOnCreateError(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such image", http.StatusNotFound)
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/containers",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/containers/create",
		StartPath:      "/v6.0.0/libpod/containers/{id}/start",
	}})
	resp := do(t, client, http.MethodPost, "/containers", strings.NewReader(`{"image":"missing"}`))
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if n := len(eng.recorded()); n != 1 {
		t.Fatalf("expected create only, hits=%d", n)
	}
}

func TestRoundTripDoesNotStartOnGET(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Id":"abc"}`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "GET",
		AdaptedPath:    "/containers/{id}",
		UpstreamMethod: "GET",
		UpstreamPath:   "/v6.0.0/libpod/containers/{name}/json",
		StartPath:      "/v6.0.0/libpod/containers/{id}/start",
	}})
	_ = do(t, client, http.MethodGet, "/containers/abc", nil)
	if n := len(eng.recorded()); n != 1 {
		t.Fatalf("GET should not start, hits=%d", n)
	}
}

func TestRoundTripArtifactPullQuery(t *testing.T) {
	eng := &fakeEngine{handler: func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ArtifactDigest":"sha256:abc"}`))
	}}
	client := newClient(t, serve(t, "unix", eng), []specadapt.Rule{{
		Method:         "POST",
		AdaptedPath:    "/artifacts",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/artifacts/pull",
		QueryFromBody:  []string{"name", "tlsVerify"},
		BodyMode:       "query",
	}})
	_ = do(t, client, http.MethodPost, "/artifacts", strings.NewReader(`{"name":"localhost/a:latest"}`))
	hits := eng.recorded()
	if hits[0].Path != "/v6.0.0/libpod/artifacts/pull" {
		t.Fatalf("path = %s", hits[0].Path)
	}
	if hits[0].Body != "" {
		t.Fatalf("body = %q", hits[0].Body)
	}
	if !strings.Contains(hits[0].Query, "name=localhost") {
		t.Fatalf("query = %s", hits[0].Query)
	}
}

func TestExtractCreateID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{`{"Id":"a"}`, "a"},
		{`{"ID":"b"}`, "b"},
		{`{"id":"c"}`, "c"},
		{`{"Name":"n"}`, "n"},
		{`{"name":"m"}`, "m"},
		{`{"Id":"","name":"fallback"}`, "fallback"},
		{`{"Id":null,"name":"n"}`, "n"},
		{`[]`, ""},
		{`not-json`, ""},
		{`{}`, ""},
	}
	for _, tc := range cases {
		if got := extractCreateID([]byte(tc.in)); got != tc.want {
			t.Errorf("extractCreateID(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
