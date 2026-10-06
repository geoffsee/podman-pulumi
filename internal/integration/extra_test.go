package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLiveImagePullNever(t *testing.T) {
	conn, client := requireEngine(t)
	ref := firstImageRef(t, conn)
	if ref == "" {
		t.Skip("no local images")
	}
	code, body := doJSON(t, client, http.MethodPost, "/images", map[string]any{
		"reference": ref,
		"policy":    "never",
	})
	if code < 200 || code >= 300 {
		t.Fatalf("pull policy=never: HTTP %d: %s", code, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("pull body should be an object, got %s: %v", body, err)
	}
	if fmt.Sprint(got["id"]) == "" && fmt.Sprint(got["Id"]) == "" {
		t.Fatalf("pull missing id: %s", body)
	}
}

func TestLiveImageTagGetDelete(t *testing.T) {
	conn, client := requireEngine(t)
	id := firstImageID(t, conn)
	if id == "" {
		t.Skip("no local images")
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	tagName := "localhost/pt-img-" + suffix + ":test"
	raw := rawClient(conn)
	code, body := doBytes(t, raw, http.MethodPost, "/v6.0.0/libpod/images/"+id+"/tag?repo=localhost/pt-img-"+suffix+"&tag=test", "", nil)
	if code < 200 || code >= 300 {
		t.Fatalf("tag: HTTP %d: %s", code, body)
	}
	cleanup(t, client, http.MethodDelete, "/images/"+tagName)

	code, body = doJSON(t, client, http.MethodGet, "/images/"+tagName, nil)
	if code != 200 {
		t.Fatalf("inspect tagged image: HTTP %d: %s", code, body)
	}
	code, body = doJSON(t, client, http.MethodDelete, "/images/"+tagName, nil)
	if code < 200 || code >= 300 {
		t.Fatalf("delete tag: HTTP %d: %s", code, body)
	}
	var deleted map[string]any
	if err := json.Unmarshal(body, &deleted); err != nil {
		t.Fatalf("image delete body should be an object, got %s: %v", body, err)
	}
	code, _ = doJSON(t, client, http.MethodGet, "/images/"+tagName, nil)
	if code != 404 {
		t.Fatalf("inspect after untag: HTTP %d", code)
	}
	code, body = doJSON(t, client, http.MethodGet, "/images/"+id, nil)
	if code != 200 {
		t.Fatalf("original image missing after untag: HTTP %d: %s", code, body)
	}
}

func TestLiveArtifactAddGetDelete(t *testing.T) {
	conn, client := requireEngine(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	name := "localhost/pt-art-" + suffix + ":latest"
	raw := rawClient(conn)
	q := url.Values{"name": {name}, "fileName": {"hello.txt"}}
	code, body := doBytes(t, raw, http.MethodPost,
		"/v6.0.0/libpod/artifacts/add?"+q.Encode(),
		"application/octet-stream",
		[]byte("hello-artifact"),
	)
	if code < 200 || code >= 300 {
		t.Fatalf("artifact add: HTTP %d: %s", code, body)
	}
	cleanup(t, client, http.MethodDelete, "/artifacts/"+name)

	code, body = doJSON(t, client, http.MethodGet, "/artifacts/"+name, nil)
	if code != 200 {
		t.Fatalf("inspect artifact: HTTP %d: %s", code, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got["Name"]) != name {
		t.Fatalf("Name = %v", got["Name"])
	}
	code, body = doJSON(t, client, http.MethodDelete, "/artifacts/"+name, nil)
	if code < 200 || code >= 300 {
		t.Fatalf("delete artifact: HTTP %d: %s", code, body)
	}
	code, _ = doJSON(t, client, http.MethodGet, "/artifacts/"+name, nil)
	if code != 404 {
		t.Fatalf("inspect after delete: HTTP %d", code)
	}
}

func TestLiveStartFailureLeavesContainer(t *testing.T) {
	conn, client := requireEngine(t)
	image := firstImageRef(t, conn)
	if image == "" {
		t.Skip("no local images")
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	name := "pt-sf-" + suffix
	cleanup(t, client, http.MethodDelete, "/containers/"+name)

	payload, err := json.Marshal(map[string]any{
		"name":    name,
		"image":   image,
		"command": []string{""},
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, "http://d/containers", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = client.Do(req)
	if err == nil || !strings.Contains(err.Error(), "start resource") {
		t.Fatalf("expected start overlay error, got %v", err)
	}

	code, body := doJSON(t, client, http.MethodGet, "/containers/"+name, nil)
	if code != 200 {
		t.Fatalf("created container should remain after start failure: HTTP %d: %s", code, body)
	}
	code, body = doJSON(t, client, http.MethodDelete, "/containers/"+name, nil)
	if code < 200 || code >= 300 {
		t.Fatalf("delete orphan: HTTP %d: %s", code, body)
	}
}
