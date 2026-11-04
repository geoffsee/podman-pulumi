package readstate

import (
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

func TestNormalizeContainerInspect(t *testing.T) {
	raw := map[string]any{
		"Name": "demo",
		"Id":   "abc",
		"Config": map[string]any{
			"Image": "docker.io/library/busybox:latest",
			"Cmd":   []any{"sleep", "infinity"},
			"Labels": map[string]any{
				"app": "1",
			},
		},
		"NetworkSettings": map[string]any{
			"Networks": map[string]any{
				"net-a": map[string]any{"IPAddress": "10.0.0.2"},
			},
		},
		"Mounts": []any{
			map[string]any{
				"Type":        "volume",
				"Name":        "data",
				"Destination": "/data",
			},
		},
	}
	got := normalizeContainerInspect(raw)
	if got["name"] != "demo" || got["image"] != "docker.io/library/busybox:latest" {
		t.Fatalf("name/image = %#v", got)
	}
	nets, _ := got["networks"].(map[string]any)
	if len(nets) != 1 || nets["net-a"] == nil {
		t.Fatalf("networks = %#v", got["networks"])
	}
	vols, _ := got["volumes"].([]any)
	if len(vols) != 1 {
		t.Fatalf("volumes = %#v", got["volumes"])
	}
	entry, _ := vols[0].(map[string]any)
	if entry["name"] != "data" {
		t.Fatalf("volume entry = %#v", entry)
	}
}

func TestFinishReadKeepsPullInputsWhenInspectOmitsThem(t *testing.T) {
	req := p.ReadRequest{
		ID: "abc",
		Inputs: property.NewMap(map[string]property.Value{
			"policy":    property.New("missing"),
			"quiet":     property.New(true),
			"reference": property.New("docker.io/library/busybox:latest"),
		}),
	}
	live := goMapToPropertyMap(map[string]any{
		"id":        "abc",
		"reference": "docker.io/library/busybox:latest",
	})
	resp := finishRead(req, live)
	for _, key := range []string{"policy", "quiet", "reference"} {
		if _, ok := resp.Properties.GetOk(key); !ok {
			t.Fatalf("missing %q after merge", key)
		}
	}
}

func TestFinishReadFillsFromPropertiesWhenLiveMissing(t *testing.T) {
	req := p.ReadRequest{
		ID: "abc",
		Properties: property.NewMap(map[string]property.Value{
			"reference": property.New("docker.io/library/busybox:latest"),
			"policy":    property.New("missing"),
		}),
	}
	live := goMapToPropertyMap(map[string]any{"id": "abc"})
	resp := finishRead(req, live)
	ref, _ := resp.Properties.GetOk("reference")
	pol, _ := resp.Properties.GetOk("policy")
	if ref.AsString() != "docker.io/library/busybox:latest" || pol.AsString() != "missing" {
		t.Fatalf("merged = %#v", resp.Properties)
	}
}
