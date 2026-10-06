package specadapt

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const pullAddUpdate = `
swagger: "2.0"
info:
  title: test
  version: "6.0.0"
paths:
  /libpod/images/pull:
    post:
      tags: [images]
      parameters:
        - in: query
          name: reference
          type: string
        - in: query
          name: quiet
          type: boolean
      responses:
        "200":
          schema:
            type: object
  /libpod/images/{name}:
    delete:
      tags: [images]
  /libpod/images/{name}/json:
    get:
      tags: [images]
  /libpod/artifacts/pull:
    post:
      tags: [artifacts]
      parameters:
        - in: query
          name: name
          required: true
          type: string
      responses:
        "200":
          schema:
            type: object
  /libpod/artifacts/add:
    post:
      tags: [artifacts]
      parameters:
        - in: query
          name: fileName
          type: string
      responses:
        "201":
          schema:
            type: object
  /libpod/artifacts/{name}:
    delete:
      tags: [artifacts]
  /libpod/artifacts/{name}/json:
    get:
      tags: [artifacts]
  /libpod/networks/create:
    post:
      tags: [networks]
      parameters:
        - in: body
          name: create
          schema:
            type: object
      responses:
        "201":
          schema:
            type: object
  /libpod/networks/{name}:
    delete:
      tags: [networks]
  /libpod/networks/{name}/json:
    get:
      tags: [networks]
  /libpod/networks/{name}/update:
    post:
      tags: [networks]
      parameters:
        - in: body
          name: update
          schema:
            type: object
  /libpod/widgets/{name}/create:
    post:
      tags: [widgets]
  /libpod/widgets/{name}:
    get:
      tags: [widgets]
  /compat/images/create:
    post:
      tags: ["images (compat)"]
`

func TestAdaptPullAddUpdateAndOrphans(t *testing.T) {
	res, err := Adapt([]byte(pullAddUpdate))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Spec, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	if _, ok := paths["/widgets/{id}"]; ok {
		t.Fatal("item-path /create should not become a resource")
	}
	if _, ok := paths["/compat/images"]; ok {
		t.Fatal("compat paths should be dropped")
	}

	byKey := map[string]Rule{}
	for _, r := range res.Rules {
		byKey[r.Method+" "+r.AdaptedPath] = r
	}

	img := byKey["POST /images"]
	if img.UpstreamPath != "/v6.0.0/libpod/images/pull" {
		t.Fatalf("image pull upstream = %+v", img)
	}
	if img.BodyMode != "query" || img.DefaultQuery["quiet"] != "true" {
		t.Fatalf("image pull rewrite = %+v", img)
	}
	if !contains(img.QueryFromBody, "reference") {
		t.Fatalf("image queryFromBody = %v", img.QueryFromBody)
	}

	art := byKey["POST /artifacts"]
	if art.UpstreamPath != "/v6.0.0/libpod/artifacts/pull" {
		t.Fatalf("artifact create should use pull, not add: %+v", art)
	}
	if art.BodyMode != "query" {
		t.Fatalf("artifact body mode = %+v", art)
	}
	if _, ok := byKey["POST /artifacts/add"]; ok {
		t.Fatal("POST /artifacts/add should be skipped when pull already owns the collection")
	}

	net := byKey["PATCH /networks/{id}"]
	if net.UpstreamMethod != "POST" || net.UpstreamPath != "/v6.0.0/libpod/networks/{name}/update" {
		t.Fatalf("network update = %+v", net)
	}

	del := byKey["DELETE /images/{id}"]
	if del.UpstreamPath != "/v6.0.0/libpod/images/{name}" {
		t.Fatalf("identity delete = %+v", del)
	}
}

func TestAdaptMissingVersionDefaultsV5(t *testing.T) {
	src := `
swagger: "2.0"
paths:
  /libpod/volumes/create:
    post:
      responses:
        "201":
          schema:
            type: object
  /libpod/volumes/{name}:
    delete: {}
  /libpod/volumes/{name}/json:
    get: {}
`
	res, err := Adapt([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res.Rules {
		if !strings.HasPrefix(r.UpstreamPath, "/v5.0.0/") {
			t.Fatalf("expected v5 fallback, got %+v", r)
		}
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
