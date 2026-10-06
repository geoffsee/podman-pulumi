package specdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	openapi "github.com/pierskarsenbarg/pulumi-openapi-provider"
)

func TestAdaptedSpecDiscoversCoreResources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, AdaptedSpec, 0o644); err != nil {
		t.Fatal(err)
	}

	schema, err := openapi.GetSchema("podman", "0.1.0", openapi.Options{
		SpecPath:  path,
		Overrides: Overrides(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		`"podman:volumes:Volume"`,
		`"podman:containers:Container"`,
		`"podman:pods:Pod"`,
		`"podman:networks:Network"`,
		`"podman:secrets:Secret"`,
		`"podman:images:Image"`,
		`"podman:artifacts:Artifact"`,
	} {
		if !strings.Contains(schema, token) {
			t.Errorf("schema missing %s", token)
		}
	}
	if strings.Contains(schema, "Quadlets") {
		t.Error("quadlets should be skipped")
	}
}
