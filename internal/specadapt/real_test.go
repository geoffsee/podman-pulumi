package specadapt

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAdaptUpstreamSwagger(t *testing.T) {
	root := filepath.Join("..", "..", "spec", "swagger.yaml")
	raw, err := os.ReadFile(root)
	if err != nil {
		t.Skip("spec/swagger.yaml not present")
	}
	res, err := Adapt(raw)
	if err != nil {
		t.Fatal(err)
	}
	need := []string{
		"POST /volumes",
		"GET /volumes/{id}",
		"DELETE /volumes/{id}",
		"POST /containers",
		"GET /containers/{id}",
		"DELETE /containers/{id}",
		"POST /pods",
		"GET /pods/{id}",
		"DELETE /pods/{id}",
		"POST /networks",
		"GET /networks/{id}",
		"DELETE /networks/{id}",
		"POST /secrets",
		"GET /secrets/{id}",
		"DELETE /secrets/{id}",
		"POST /images",
		"GET /images/{id}",
		"DELETE /images/{id}",
		"POST /artifacts",
		"GET /artifacts/{id}",
		"DELETE /artifacts/{id}",
	}
	have := map[string]Rule{}
	for _, r := range res.Rules {
		have[r.Method+" "+r.AdaptedPath] = r
	}
	for _, key := range need {
		if _, ok := have[key]; !ok {
			t.Errorf("missing rewritten operation %s", key)
		}
	}
	if have["POST /containers"].UpstreamPath != "/v6.0.0/libpod/containers/create" {
		t.Errorf("container create upstream = %s", have["POST /containers"].UpstreamPath)
	}
	if have["GET /volumes/{id}"].UpstreamPath != "/v6.0.0/libpod/volumes/{name}/json" {
		t.Errorf("volume inspect upstream = %s", have["GET /volumes/{id}"].UpstreamPath)
	}
	for _, drop := range []string{"POST /artifacts/local", "GET /exec/{id}", "GET /manifests/{id}", "PATCH /containers/{id}"} {
		if _, ok := have[drop]; ok {
			t.Errorf("leftover operation %s should be pruned", drop)
		}
	}

	img := have["POST /images"]
	if img.BodyMode != "query" || img.DefaultQuery["quiet"] != "true" {
		t.Errorf("image pull rewrite: %+v", img)
	}
	if have["POST /pods"].StartPath != "/v6.0.0/libpod/pods/{id}/start" {
		t.Errorf("pod start overlay = %s", have["POST /pods"].StartPath)
	}
	netPatch := have["PATCH /networks/{id}"]
	if netPatch.UpstreamMethod != "POST" || netPatch.UpstreamPath != "/v6.0.0/libpod/networks/{name}/update" {
		t.Errorf("network patch = %+v", netPatch)
	}
	if have["POST /secrets"].BodyMode != "rawString" {
		t.Errorf("secret body mode = %+v", have["POST /secrets"])
	}
	art := have["POST /artifacts"]
	if art.UpstreamPath != "/v6.0.0/libpod/artifacts/pull" || art.BodyMode != "query" {
		t.Errorf("artifact pull rewrite: %+v", art)
	}
}

func TestCommittedArtifactsMatchAdapt(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "spec", "swagger.yaml"))
	if err != nil {
		t.Skip("spec/swagger.yaml not present")
	}
	res, err := Adapt(raw)
	if err != nil {
		t.Fatal(err)
	}
	committedMap, err := os.ReadFile(filepath.Join(root, "internal", "specdata", "pathmap.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.PathMap, committedMap) {
		t.Fatal("internal/specdata/pathmap.json is stale; run make generate-local")
	}
	committedSpec, err := os.ReadFile(filepath.Join(root, "internal", "specdata", "openapi.adapted.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.Spec, committedSpec) {
		t.Fatal("internal/specdata/openapi.adapted.yaml is stale; run make generate-local")
	}
}
