package specadapt

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const mini = `
swagger: "2.0"
info:
  title: test
  version: "6.0.0"
host: podman.io
paths:
  /libpod/volumes/create:
    post:
      tags: [volumes]
      parameters:
        - in: body
          name: create
          schema:
            $ref: "#/definitions/VolumeCreateOptions"
      responses:
        "201":
          schema:
            $ref: "#/definitions/VolumeConfigResponse"
  /libpod/volumes/{name}:
    delete:
      tags: [volumes]
      parameters:
        - in: path
          name: name
          required: true
          type: string
  /libpod/volumes/{name}/json:
    get:
      tags: [volumes]
      parameters:
        - in: path
          name: name
          required: true
          type: string
      responses:
        "200":
          schema:
            $ref: "#/definitions/VolumeConfigResponse"
  /libpod/volumes/{name}/exists:
    get:
      tags: [volumes]
  /libpod/containers/create:
    post:
      tags: [containers]
      parameters:
        - in: body
          name: create
          schema:
            $ref: "#/definitions/SpecGenerator"
      responses:
        "201":
          schema:
            $ref: "#/definitions/ContainerCreateResponse"
  /libpod/containers/{name}:
    delete:
      tags: [containers]
  /libpod/containers/{name}/json:
    get:
      tags: [containers]
      responses:
        "200":
          schema:
            $ref: "#/definitions/InspectContainerData"
  /libpod/containers/{name}/update:
    post:
      tags: [containers]
  /libpod/containers/{name}/start:
    post:
      tags: [containers]
  /libpod/secrets/create:
    post:
      tags: [secrets]
      parameters:
        - in: query
          name: name
          required: true
          type: string
        - in: body
          name: request
          schema:
            type: string
      responses:
        "201":
          schema:
            properties:
              ID:
                type: string
  /libpod/secrets/{name}:
    delete:
      tags: [secrets]
  /libpod/secrets/{name}/json:
    get:
      tags: [secrets]
  /containers/json:
    get:
      tags: ["containers (compat)"]
  /libpod/exec/{id}/json:
    get:
      tags: [exec]
      parameters:
        - in: path
          name: id
          required: true
          type: string
definitions:
  VolumeCreateOptions:
    type: object
    properties:
      Name:
        type: string
  VolumeConfigResponse:
    type: object
    properties:
      Name:
        type: string
  SpecGenerator:
    type: object
    properties:
      image:
        type: string
      name:
        type: string
  ContainerCreateResponse:
    type: object
    properties:
      Id:
        type: string
  InspectContainerData:
    type: object
    properties:
      Id:
        type: string
`

func TestAdaptMiniSpec(t *testing.T) {
	res, err := Adapt([]byte(mini))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Spec, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	if _, ok := paths["/volumes"]; !ok {
		t.Fatalf("missing /volumes, got %v", keys(paths))
	}
	if _, ok := paths["/volumes/{id}"]; !ok {
		t.Fatalf("missing /volumes/{id}, got %v", keys(paths))
	}
	if _, ok := paths["/containers/{id}"]; !ok {
		t.Fatalf("missing /containers/{id}")
	}
	if _, ok := paths["/libpod/volumes/create"]; ok {
		t.Fatal("upstream create path leaked into adapted spec")
	}
	if _, ok := paths["/containers/json"]; ok {
		t.Fatal("compat path leaked")
	}
	if _, ok := paths["/exec/{id}"]; ok {
		t.Fatal("orphan inspect-only exec path should be dropped")
	}

	vol := paths["/volumes"].(map[string]any)
	if _, ok := vol["post"]; !ok {
		t.Fatal("volumes collection missing POST")
	}
	item := paths["/volumes/{id}"].(map[string]any)
	if _, ok := item["get"]; !ok {
		t.Fatal("volume item missing GET")
	}
	if _, ok := item["delete"]; !ok {
		t.Fatal("volume item missing DELETE")
	}
	ctr := paths["/containers/{id}"].(map[string]any)
	if _, ok := ctr["patch"]; ok {
		t.Fatal("container /update should not be exposed as resource PATCH")
	}
	if _, ok := ctr["post"]; ok {
		t.Fatal("container item should not keep POST start")
	}

	secret := paths["/secrets"].(map[string]any)
	post := secret["post"].(map[string]any)
	params := post["parameters"].([]any)
	foundData := false
	for _, raw := range params {
		pm := raw.(map[string]any)
		if fmt.Sprint(pm["in"]) != "body" {
			continue
		}
		schema := pm["schema"].(map[string]any)
		props := schema["properties"].(map[string]any)
		if _, ok := props["data"]; ok {
			foundData = true
		}
		if _, ok := props["name"]; !ok {
			t.Fatal("secret body missing name query promotion")
		}
	}
	if !foundData {
		t.Fatal("secret body missing data field")
	}

	byKey := map[string]Rule{}
	for _, r := range res.Rules {
		byKey[r.Method+" "+r.AdaptedPath] = r
	}
	got := byKey["POST /volumes"]
	if got.UpstreamPath != "/v6.0.0/libpod/volumes/create" {
		t.Fatalf("volume create map: %+v", got)
	}
	got = byKey["GET /volumes/{id}"]
	if got.UpstreamPath != "/v6.0.0/libpod/volumes/{name}/json" {
		t.Fatalf("volume inspect map: %+v", got)
	}
	got = byKey["POST /containers"]
	if got.StartPath != "/v6.0.0/libpod/containers/{id}/start" {
		t.Fatalf("container start overlay: %+v", got)
	}
	got = byKey["DELETE /containers/{id}"]
	if got.DefaultQuery["force"] != "true" {
		t.Fatalf("container delete force: %+v", got)
	}
	got = byKey["POST /secrets"]
	if got.BodyMode != "rawString" {
		t.Fatalf("secret body mode: %+v", got)
	}
	got = byKey["DELETE /volumes/{id}"]
	if got.UpstreamPath != "/v6.0.0/libpod/volumes/{name}" {
		t.Fatalf("identity volume delete: %+v", got)
	}

	for p := range paths {
		if strings.Contains(p, "exists") {
			t.Fatalf("non-CRUD exists path leaked: %s", p)
		}
	}
}

func TestAPIPrefix(t *testing.T) {
	cases := map[string]string{
		"6.0.0":    "/v6.0.0",
		"v5.2.1":   "/v5.0.0",
		"":         "/v5.0.0",
		"4":        "/v4.0.0",
		"v6":       "/v6.0.0",
		"<nil>":    "/v5.0.0",
		"  6.1.2 ": "/v6.0.0",
		".":        "/v5.0.0",
	}
	for in, want := range cases {
		if got := APIPrefix(in); got != want {
			t.Errorf("APIPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAdaptInvalidYAML(t *testing.T) {
	_, err := Adapt([]byte("{this is not yaml"))
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestAdaptMissingPaths(t *testing.T) {
	_, err := Adapt([]byte("swagger: '2.0'\ninfo:\n  version: '6.0.0'\n"))
	if err == nil || !strings.Contains(err.Error(), "no paths") {
		t.Fatalf("err = %v", err)
	}
}

func TestAdaptDropsHost(t *testing.T) {
	res, err := Adapt([]byte(mini))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.Spec), "host: podman.io") {
		t.Fatal("adapted spec still contains upstream host")
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
