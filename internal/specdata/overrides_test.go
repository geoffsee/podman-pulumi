package specdata

import (
	"encoding/json"
	"testing"

	"github.com/geoffsee/pulumi-podman/internal/specadapt"
)

func TestOverridesTokensAndIDFields(t *testing.T) {
	o := Overrides()
	want := map[string]struct {
		token   string
		idField string
		skip    bool
	}{
		"Volumes":    {token: "podman:volumes:Volume", idField: "Name"},
		"Containers": {token: "podman:containers:Container", idField: "Id"},
		"Pods":       {token: "podman:pods:Pod", idField: "Id"},
		"Networks":   {token: "podman:networks:Network", idField: "id"},
		"Secrets":    {token: "podman:secrets:Secret", idField: "ID"},
		"Images":     {token: "podman:images:Image", idField: "id"},
		"Artifacts":  {token: "podman:artifacts:Artifact"},
		"Quadlets":   {skip: true},
	}
	for name, w := range want {
		got, ok := o[name]
		if !ok {
			t.Errorf("missing override %s", name)
			continue
		}
		if got.Token != w.token || got.IDField != w.idField || got.Skip != w.skip {
			t.Errorf("%s = token=%q id=%q skip=%v", name, got.Token, got.IDField, got.Skip)
		}
	}
	if o["*"].Diff == nil {
		t.Fatal("wildcard Diff hook missing")
	}
	if o["Containers"].Diff == nil || o["Pods"].Diff == nil {
		t.Fatal("container/pod replace Diff hook missing")
	}
}

func TestPathMapJSON(t *testing.T) {
	var rules []specadapt.Rule
	if err := json.Unmarshal(PathMap, &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) == 0 {
		t.Fatal("empty path map")
	}
	byKey := map[string]specadapt.Rule{}
	for _, r := range rules {
		byKey[r.Method+" "+r.AdaptedPath] = r
	}
	if byKey["POST /containers"].StartPath != "/v6.0.0/libpod/containers/{id}/start" {
		t.Fatalf("container start = %+v", byKey["POST /containers"])
	}
	if byKey["POST /pods"].StartPath != "/v6.0.0/libpod/pods/{id}/start" {
		t.Fatalf("pod start = %+v", byKey["POST /pods"])
	}
	if byKey["DELETE /containers/{id}"].DefaultQuery["force"] != "true" {
		t.Fatalf("container delete = %+v", byKey["DELETE /containers/{id}"])
	}
	if byKey["POST /images"].BodyMode != "query" || byKey["POST /images"].DefaultQuery["quiet"] != "true" {
		t.Fatalf("image pull = %+v", byKey["POST /images"])
	}
	if byKey["POST /secrets"].BodyMode != "rawString" {
		t.Fatalf("secrets = %+v", byKey["POST /secrets"])
	}
	if _, ok := byKey["PATCH /containers/{id}"]; ok {
		t.Fatal("container PATCH should not be in the path map")
	}
	art := byKey["POST /artifacts"]
	if art.BodyMode != "query" || art.UpstreamPath != "/v6.0.0/libpod/artifacts/pull" {
		t.Fatalf("artifact pull = %+v", art)
	}
	if byKey["DELETE /pods/{id}"].DefaultQuery["force"] != "true" {
		t.Fatalf("pod delete = %+v", byKey["DELETE /pods/{id}"])
	}
	if byKey["DELETE /containers/{id}"].DefaultQuery["v"] != "true" {
		t.Fatalf("container delete volumes = %+v", byKey["DELETE /containers/{id}"])
	}
}
