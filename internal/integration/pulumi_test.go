package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

var (
	providerOnce sync.Once
	providerBin  string
	providerErr  error
)

func requirePulumi(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pulumi"); err != nil {
		t.Skip("pulumi CLI not on PATH")
	}
}

func builtProvider(t *testing.T) string {
	t.Helper()
	requirePulumi(t)
	providerOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pulumi-podman-bin-*")
		if err != nil {
			providerErr = err
			return
		}
		pluginDir := filepath.Join(dir, "podman")
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
			providerErr = err
			return
		}
		out := filepath.Join(pluginDir, "pulumi-resource-podman")
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = repoRoot()
		if b, err := cmd.CombinedOutput(); err != nil {
			providerErr = fmt.Errorf("go build: %w\n%s", err, b)
			return
		}
		providerBin = out
	})
	if providerErr != nil {
		t.Fatal(providerErr)
	}
	return providerBin
}

func pulumiEnv(dir string) []string {
	return append(os.Environ(),
		"PULUMI_BACKEND_URL=file://"+filepath.ToSlash(filepath.Join(dir, ".pulumi-state")),
		"PULUMI_CONFIG_PASSPHRASE=pulumi-podman-test",
		"PULUMI_SKIP_UPDATE_CHECK=true",
	)
}

func runPulumi(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "pulumi", args...)
	cmd.Dir = dir
	cmd.Env = pulumiEnv(dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pulumi %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func writeVolumeProgram(t *testing.T, dir, providerPath, volumeName string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".pulumi-state"), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := fmt.Sprintf(`name: pt-pulumi
runtime: yaml
plugins:
  providers:
    - name: podman
      path: %s
resources:
  vol:
    type: podman:volumes:Volume
    properties:
      name: %s
      labels:
        pulumi-podman-test: "1"
`, filepath.ToSlash(providerPath), volumeName)
	if err := os.WriteFile(filepath.Join(dir, "Pulumi.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProviderGetSchema(t *testing.T) {
	bin := builtProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pulumi", "package", "get-schema", bin)
	cmd.Env = append(os.Environ(), "PULUMI_SKIP_UPDATE_CHECK=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("get-schema: %v\n%s", err, out)
	}
	var schema map[string]any
	if err := json.Unmarshal(out, &schema); err != nil {
		t.Fatalf("schema json: %v\n%s", err, out)
	}
	resources, _ := schema["resources"].(map[string]any)
	for _, token := range []string{
		"podman:volumes:Volume",
		"podman:containers:Container",
		"podman:pods:Pod",
		"podman:networks:Network",
		"podman:secrets:Secret",
		"podman:images:Image",
		"podman:artifacts:Artifact",
	} {
		if _, ok := resources[token]; !ok {
			t.Errorf("schema missing %s", token)
		}
	}
	for k := range resources {
		if strings.Contains(strings.ToLower(k), "quadlet") {
			t.Errorf("quadlets should be skipped, found %s", k)
		}
	}
	if fmt.Sprint(schema["name"]) != "podman" {
		t.Fatalf("name = %v", schema["name"])
	}
	if fmt.Sprint(schema["displayName"]) != "Podman" {
		t.Errorf("displayName = %v", schema["displayName"])
	}
	if fmt.Sprint(schema["publisher"]) != "geoffsee" {
		t.Errorf("publisher = %v", schema["publisher"])
	}
	if fmt.Sprint(schema["pluginDownloadURL"]) != "github://api.github.com/geoffsee/pulumi-podman" {
		t.Errorf("pluginDownloadURL = %v", schema["pluginDownloadURL"])
	}
	if fmt.Sprint(schema["logoUrl"]) != "https://raw.githubusercontent.com/geoffsee/pulumi-podman/main/docs/logo.svg" {
		t.Errorf("logoUrl = %v", schema["logoUrl"])
	}
	keywords, _ := schema["keywords"].([]any)
	got := map[string]bool{}
	for _, k := range keywords {
		got[fmt.Sprint(k)] = true
	}
	for _, want := range []string{"category/infrastructure", "kind/native"} {
		if !got[want] {
			t.Errorf("keywords missing %s: %v", want, keywords)
		}
	}
}

func TestProviderGetSchemaWithoutEngine(t *testing.T) {
	bin := builtProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pulumi", "package", "get-schema", bin)
	cmd.Env = append(os.Environ(),
		"PULUMI_SKIP_UPDATE_CHECK=true",
		"CONTAINER_HOST=ssh://unused",
		"PODMAN_HOST=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("get-schema without engine: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"podman:volumes:Volume"`) {
		t.Fatalf("schema missing volume token: %s", out[:min(len(out), 500)])
	}
}

func TestPulumiVolumeUpRefreshDestroy(t *testing.T) {
	_, client := requireEngine(t)
	bin := builtProvider(t)
	pluginDir := filepath.Dir(bin)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	name := "pt-pulumi-vol-" + suffix
	dir := t.TempDir()
	writeVolumeProgram(t, dir, pluginDir, name)
	cleanup(t, client, http.MethodDelete, "/volumes/"+name)

	runPulumi(t, dir, "stack", "init", "dev", "--non-interactive")
	up := runPulumi(t, dir, "up", "--yes", "--non-interactive", "--skip-preview")
	if !strings.Contains(up, name) && !strings.Contains(up, "created") && !strings.Contains(strings.ToLower(up), "resources") {
		t.Logf("up output:\n%s", up)
	}
	code, body := doJSON(t, client, http.MethodGet, "/volumes/"+name, nil)
	if code != 200 {
		t.Fatalf("volume missing after pulumi up: HTTP %d: %s", code, body)
	}
	preview := runPulumi(t, dir, "preview", "--non-interactive", "--expect-no-changes")
	if strings.Contains(preview, "error") && strings.Contains(strings.ToLower(preview), "expected no changes") {
		t.Fatalf("preview reported changes:\n%s", preview)
	}
	runPulumi(t, dir, "refresh", "--yes", "--non-interactive", "--expect-no-changes")
	runPulumi(t, dir, "destroy", "--yes", "--non-interactive", "--skip-preview")
	code, _ = doJSON(t, client, http.MethodGet, "/volumes/"+name, nil)
	if code != 404 {
		t.Fatalf("volume still present after destroy: HTTP %d", code)
	}
}

func TestPulumiVolumeImport(t *testing.T) {
	_, client := requireEngine(t)
	bin := builtProvider(t)
	pluginDir := filepath.Dir(bin)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	name := "pt-pulumi-imp-" + suffix
	cleanup(t, client, http.MethodDelete, "/volumes/"+name)
	code, body := doJSON(t, client, http.MethodPost, "/volumes", map[string]any{
		"Name":   name,
		"Labels": map[string]string{"pulumi-podman-test": "1"},
	})
	if code < 200 || code >= 300 {
		t.Fatalf("create volume: HTTP %d: %s", code, body)
	}

	dir := t.TempDir()
	writeVolumeProgram(t, dir, pluginDir, name)
	runPulumi(t, dir, "stack", "init", "dev", "--non-interactive")
	runPulumi(t, dir, "import", "podman:volumes:Volume", "vol", name,
		"--yes", "--non-interactive", "--skip-preview",
		"--protect=false", "--generate-code=false")
	runPulumi(t, dir, "preview", "--non-interactive", "--expect-no-changes")
	runPulumi(t, dir, "destroy", "--yes", "--non-interactive", "--skip-preview")
	code, _ = doJSON(t, client, http.MethodGet, "/volumes/"+name, nil)
	if code != 404 {
		t.Fatalf("imported volume still present after destroy: HTTP %d", code)
	}
}
