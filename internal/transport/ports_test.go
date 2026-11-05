package transport

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/geoffsee/pulumi-podman/internal/specadapt"
)

func TestContainerCreateTranslatesNestedPortMappings(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://d/containers", strings.NewReader(`{
		"portmappings":[{"containerPort":80,"hostPort":8080,"hostIp":"127.0.0.1","protocol":"tcp","range":1},
		{"container_port":443,"host_port":8443,"protocol":"tcp"}],
		"labels":{"hostPort":"do-not-rename"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	err = ApplyRule(req, specadapt.Rule{UpstreamMethod: "POST", UpstreamPath: "/v5.0.0/libpod/containers/create"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Ports  []map[string]any  `json:"portmappings"`
		Labels map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Ports) != 2 {
		t.Fatalf("ports = %#v", body.Ports)
	}
	first := body.Ports[0]
	if first["container_port"] != float64(80) || first["host_port"] != float64(8080) || first["host_ip"] != "127.0.0.1" || first["protocol"] != "tcp" || first["range"] != float64(1) {
		t.Fatalf("incorrect API port mapping: %#v", first)
	}
	for _, key := range []string{"containerPort", "hostPort", "hostIp"} {
		if _, exists := first[key]; exists {
			t.Fatalf("SDK key %s leaked into API body", key)
		}
	}
	if body.Ports[1]["container_port"] != float64(443) || body.Labels["hostPort"] != "do-not-rename" {
		t.Fatalf("unrelated fields changed: %#v", body)
	}
}
