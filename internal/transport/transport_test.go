package transport

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/geoffsee/pulumi-podman/internal/specadapt"
)

func TestApplyRuleVolumeInspect(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://d/volumes/data", nil)
	err := ApplyRule(req, specadapt.Rule{
		Method:         "GET",
		AdaptedPath:    "/volumes/{id}",
		UpstreamMethod: "GET",
		UpstreamPath:   "/libpod/volumes/{name}/json",
	}, map[string]string{"id": "data"})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/libpod/volumes/data/json" {
		t.Fatalf("path = %s", req.URL.Path)
	}
}

func TestApplyRuleSecretRawBody(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://d/secrets", strings.NewReader(`{"name":"db","data":"s3cret"}`))
	req.Header.Set("Content-Type", "application/json")
	err := ApplyRule(req, specadapt.Rule{
		Method:         "POST",
		AdaptedPath:    "/secrets",
		UpstreamMethod: "POST",
		UpstreamPath:   "/libpod/secrets/create",
		BodyMode:       "rawString",
		RawStringField: "data",
		QueryFromBody:  []string{"name"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/libpod/secrets/create" {
		t.Fatalf("path = %s", req.URL.Path)
	}
	if req.URL.Query().Get("name") != "db" {
		t.Fatalf("query = %s", req.URL.RawQuery)
	}
	raw, _ := io.ReadAll(req.Body)
	if string(raw) != "s3cret" {
		t.Fatalf("body = %q", raw)
	}
}

func TestCoerceJSONObject(t *testing.T) {
	if got := string(coerceJSONObject([]byte(`[{"Ok":true}]`))); got != "{}" {
		t.Fatalf("array = %q", got)
	}
	obj := []byte(`{"Id":"abc"}`)
	if got := string(coerceJSONObject(obj)); got != string(obj) {
		t.Fatalf("object = %q", got)
	}
	if got := string(coerceJSONObject(nil)); got != "" {
		t.Fatalf("empty = %q", got)
	}
}

func TestRewriteAPIPrefix(t *testing.T) {
	rules := []specadapt.Rule{{
		UpstreamPath: "/v6.0.0/libpod/volumes/create",
		StartPath:    "/v6.0.0/libpod/containers/{id}/start",
	}}
	got := RewriteAPIPrefix(rules, "5.0.0")
	if got[0].UpstreamPath != "/v5.0.0/libpod/volumes/create" {
		t.Fatalf("upstream = %s", got[0].UpstreamPath)
	}
	if got[0].StartPath != "/v5.0.0/libpod/containers/{id}/start" {
		t.Fatalf("start = %s", got[0].StartPath)
	}
	unversioned := RewriteAPIPrefix([]specadapt.Rule{{UpstreamPath: "/libpod/volumes/create"}}, "6.0.0")
	if unversioned[0].UpstreamPath != "/v6.0.0/libpod/volumes/create" {
		t.Fatalf("unversioned = %s", unversioned[0].UpstreamPath)
	}
}

func TestApplyRuleDeleteForce(t *testing.T) {
	req, _ := http.NewRequest(http.MethodDelete, "http://d/containers/abc", nil)
	err := ApplyRule(req, specadapt.Rule{
		Method:         "DELETE",
		AdaptedPath:    "/containers/{id}",
		UpstreamMethod: "DELETE",
		UpstreamPath:   "/libpod/containers/{name}",
		DefaultQuery:   map[string]string{"force": "true"},
	}, map[string]string{"id": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/libpod/containers/abc" {
		t.Fatalf("path = %s", req.URL.Path)
	}
	if req.URL.Query().Get("force") != "true" {
		t.Fatalf("query = %s", req.URL.RawQuery)
	}
}

func TestApplyRuleDefaultQueryDoesNotOverride(t *testing.T) {
	req, _ := http.NewRequest(http.MethodDelete, "http://d/containers/abc?force=false", nil)
	err := ApplyRule(req, specadapt.Rule{
		Method:         "DELETE",
		AdaptedPath:    "/containers/{id}",
		UpstreamMethod: "DELETE",
		UpstreamPath:   "/libpod/containers/{name}",
		DefaultQuery:   map[string]string{"force": "true"},
	}, map[string]string{"id": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Query().Get("force") != "false" {
		t.Fatalf("query = %s", req.URL.RawQuery)
	}
}

func TestApplyRuleImagePullQuery(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://d/images", strings.NewReader(`{"reference":"busybox:latest","quiet":false}`))
	err := ApplyRule(req, specadapt.Rule{
		Method:         "POST",
		AdaptedPath:    "/images",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/images/pull",
		QueryFromBody:  []string{"reference", "quiet"},
		DefaultQuery:   map[string]string{"quiet": "true"},
		BodyMode:       "query",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v6.0.0/libpod/images/pull" {
		t.Fatalf("path = %s", req.URL.Path)
	}
	if req.URL.Query().Get("reference") != "busybox:latest" {
		t.Fatalf("reference = %s", req.URL.RawQuery)
	}
	if req.URL.Query().Get("quiet") != "false" {
		t.Fatalf("body quiet should win over default: %s", req.URL.RawQuery)
	}
	if req.Body != http.NoBody {
		raw, _ := io.ReadAll(req.Body)
		t.Fatalf("expected empty body, got %q", raw)
	}
}

func TestApplyRuleKeepsJSONBody(t *testing.T) {
	orig := `{"Name":"data","Labels":{"a":"b"}}`
	req, _ := http.NewRequest(http.MethodPost, "http://d/volumes", strings.NewReader(orig))
	req.Header.Set("Content-Type", "application/json")
	err := ApplyRule(req, specadapt.Rule{
		Method:         "POST",
		AdaptedPath:    "/volumes",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/volumes/create",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(req.Body)
	if string(raw) != orig {
		t.Fatalf("body = %s", raw)
	}
	if req.URL.Path != "/v6.0.0/libpod/volumes/create" {
		t.Fatalf("path = %s", req.URL.Path)
	}
}

func TestApplyRuleNameParam(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://d/volumes/data", nil)
	err := ApplyRule(req, specadapt.Rule{
		Method:         "GET",
		AdaptedPath:    "/volumes/{id}",
		UpstreamMethod: "GET",
		UpstreamPath:   "/libpod/volumes/{name}/json",
	}, map[string]string{"name": "data"})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/libpod/volumes/data/json" {
		t.Fatalf("path = %s", req.URL.Path)
	}
}

func TestApplyRuleInvalidJSONQueryMode(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://d/images", strings.NewReader(`not-json`))
	err := ApplyRule(req, specadapt.Rule{
		Method:         "POST",
		AdaptedPath:    "/images",
		UpstreamMethod: "POST",
		UpstreamPath:   "/v6.0.0/libpod/images/pull",
		BodyMode:       "query",
		QueryFromBody:  []string{"reference"},
		DefaultQuery:   map[string]string{"quiet": "true"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v6.0.0/libpod/images/pull" {
		t.Fatalf("path = %s", req.URL.Path)
	}
	if req.URL.Query().Get("quiet") != "true" {
		t.Fatalf("query = %s", req.URL.RawQuery)
	}
}

func TestApplyRuleRawStringDefaultField(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://d/secrets", strings.NewReader(`{"name":"db","data":"s3cret"}`))
	err := ApplyRule(req, specadapt.Rule{
		Method:         "POST",
		AdaptedPath:    "/secrets",
		UpstreamMethod: "POST",
		UpstreamPath:   "/libpod/secrets/create",
		BodyMode:       "rawString",
		QueryFromBody:  []string{"name"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(req.Body)
	if string(raw) != "s3cret" {
		t.Fatalf("body = %q", raw)
	}
}

func TestApplyRuleQueryFromBodyKeepsRemainingJSON(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://d/x", strings.NewReader(`{"name":"n","Labels":{"a":"b"}}`))
	err := ApplyRule(req, specadapt.Rule{
		Method:         "POST",
		AdaptedPath:    "/x",
		UpstreamMethod: "POST",
		UpstreamPath:   "/libpod/x/create",
		QueryFromBody:  []string{"name"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Query().Get("name") != "n" {
		t.Fatalf("query = %s", req.URL.RawQuery)
	}
	raw, _ := io.ReadAll(req.Body)
	if !strings.Contains(string(raw), `"Labels"`) || strings.Contains(string(raw), `"name"`) {
		t.Fatalf("body = %s", raw)
	}
	if req.GetBody == nil {
		t.Fatal("GetBody should be set after rewrite")
	}
	again, err := req.GetBody()
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := io.ReadAll(again)
	if string(raw2) != string(raw) {
		t.Fatalf("GetBody = %s", raw2)
	}
}

func TestApplyRuleExtraPathParams(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://d/v/6/volumes/data", nil)
	err := ApplyRule(req, specadapt.Rule{
		Method:         "GET",
		AdaptedPath:    "/v/{ver}/volumes/{id}",
		UpstreamMethod: "GET",
		UpstreamPath:   "/v{ver}.0.0/libpod/volumes/{name}/json",
	}, map[string]string{"ver": "6", "id": "data"})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v6.0.0/libpod/volumes/data/json" {
		t.Fatalf("path = %s", req.URL.Path)
	}
}

func TestApplyRuleSlashID(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://d/images/localhost/busybox:latest", nil)
	err := ApplyRule(req, specadapt.Rule{
		Method:         "GET",
		AdaptedPath:    "/images/{id}",
		UpstreamMethod: "GET",
		UpstreamPath:   "/v6.0.0/libpod/images/{name}/json",
	}, map[string]string{"id": "localhost/busybox:latest"})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v6.0.0/libpod/images/localhost/busybox:latest/json" {
		t.Fatalf("path = %s", req.URL.Path)
	}
}

func TestCoerceJSONObjectEmptyArray(t *testing.T) {
	if got := string(coerceJSONObject([]byte(`[]`))); got != "{}" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeResponseNilBody(t *testing.T) {
	resp, err := normalizeResponse(&http.Response{StatusCode: 204})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Body != nil {
		t.Fatal("nil body should stay nil")
	}
}
