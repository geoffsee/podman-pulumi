package pconfig

import (
	"encoding/json"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
)

func TestPatchSchemaAddsConnectionVariables(t *testing.T) {
	raw := `{"name":"podman","config":{"variables":{"baseUrl":{"type":"string"}}}}`
	out, err := patchSchema(raw)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	vars := doc["config"].(map[string]any)["variables"].(map[string]any)
	if _, ok := vars["baseUrl"]; !ok {
		t.Fatal("expected baseUrl preserved")
	}
	if _, ok := vars["connectionUri"]; !ok {
		t.Fatal("expected connectionUri added")
	}
	if _, ok := vars["sshPrivateKey"].(map[string]any)["secret"]; !ok {
		t.Fatal("expected sshPrivateKey secret")
	}
}

func TestSourceFromConfigure(t *testing.T) {
	req := p.ConfigureRequest{
		Variables: map[string]string{
			"connectionUri":   "ssh://user@host/run/podman/podman.sock",
			"sshIdentityFile": "/tmp/key",
		},
	}
	src := SourceFromConfigure(req)
	if src.ConnectionURI != req.Variables["connectionUri"] {
		t.Fatalf("ConnectionURI = %q", src.ConnectionURI)
	}
	if src.SSHIdentityFile != req.Variables["sshIdentityFile"] {
		t.Fatalf("SSHIdentityFile = %q", src.SSHIdentityFile)
	}
}
