package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/geoffsee/pulumi-podman/internal/specadapt"
	"github.com/geoffsee/pulumi-podman/internal/specdata"
	"github.com/geoffsee/pulumi-podman/internal/transport"
)

func requireEngine(t *testing.T) (transport.Conn, *http.Client) {
	t.Helper()
	conn, err := transport.DetectConn()
	if err != nil {
		t.Skip(err)
	}
	if err := pingEngine(conn); err != nil {
		t.Skip(err)
	}
	var rules []specadapt.Rule
	if err := json.Unmarshal(specdata.PathMap, &rules); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PODMAN_API_VERSION", "")
	return conn, transport.New(conn, rules)
}

func TestLiveEngineCRUD(t *testing.T) {
	conn, client := requireEngine(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)

	t.Run("volume", func(t *testing.T) {
		name := "pt-vol-" + suffix
		cleanup(t, client, http.MethodDelete, "/volumes/"+name)
		code, body := doJSON(t, client, http.MethodPost, "/volumes", map[string]any{
			"Name":   name,
			"Labels": map[string]string{"pulumi-podman-test": "1"},
		})
		if code < 200 || code >= 300 {
			t.Fatalf("create volume: HTTP %d: %s", code, body)
		}
		code, body = doJSON(t, client, http.MethodGet, "/volumes/"+name, nil)
		if code != 200 {
			t.Fatalf("inspect volume: HTTP %d: %s", code, body)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(got["Name"]) != name {
			t.Fatalf("Name = %v", got["Name"])
		}
		labels, _ := got["Labels"].(map[string]any)
		if fmt.Sprint(labels["pulumi-podman-test"]) != "1" {
			t.Fatalf("Labels = %v", got["Labels"])
		}
		code, body = doJSON(t, client, http.MethodDelete, "/volumes/"+name, nil)
		if code < 200 || code >= 300 {
			t.Fatalf("delete volume: HTTP %d: %s", code, body)
		}
		code, _ = doJSON(t, client, http.MethodGet, "/volumes/"+name, nil)
		if code != 404 {
			t.Fatalf("inspect after delete: HTTP %d", code)
		}
	})

	t.Run("secret", func(t *testing.T) {
		name := "pt-sec-" + suffix
		cleanup(t, client, http.MethodDelete, "/secrets/"+name)
		code, body := doJSON(t, client, http.MethodPost, "/secrets", map[string]any{
			"name": name,
			"data": "s3cret-value",
		})
		if code < 200 || code >= 300 {
			t.Fatalf("create secret: HTTP %d: %s", code, body)
		}
		var created map[string]any
		if err := json.Unmarshal(body, &created); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(created["ID"]) == "" && fmt.Sprint(created["Id"]) == "" {
			t.Fatalf("create secret missing ID: %s", body)
		}
		code, body = doJSON(t, client, http.MethodGet, "/secrets/"+name, nil)
		if code != 200 {
			t.Fatalf("inspect secret: HTTP %d: %s", code, body)
		}
		code, body = doJSON(t, client, http.MethodDelete, "/secrets/"+name, nil)
		if code < 200 || code >= 300 {
			t.Fatalf("delete secret: HTTP %d: %s", code, body)
		}
	})

	t.Run("network", func(t *testing.T) {
		name := "pt-net-" + suffix
		cleanup(t, client, http.MethodDelete, "/networks/"+name)
		code, body := doJSON(t, client, http.MethodPost, "/networks", map[string]any{
			"name":        name,
			"dns_enabled": true,
		})
		if code < 200 || code >= 300 {
			t.Fatalf("create network: HTTP %d: %s", code, body)
		}
		code, body = doJSON(t, client, http.MethodGet, "/networks/"+name, nil)
		if code != 200 {
			t.Fatalf("inspect network: HTTP %d: %s", code, body)
		}
		code, body = doJSON(t, client, http.MethodPatch, "/networks/"+name, map[string]any{
			"adddnsservers": []string{"1.1.1.1"},
		})
		if code < 200 || code >= 300 {
			t.Fatalf("patch network: HTTP %d: %s", code, body)
		}
		code, body = doJSON(t, client, http.MethodDelete, "/networks/"+name, nil)
		if code < 200 || code >= 300 {
			t.Fatalf("delete network: HTTP %d: %s", code, body)
		}
	})

	t.Run("pod", func(t *testing.T) {
		name := "pt-pod-" + suffix
		cleanup(t, client, http.MethodDelete, "/pods/"+name)
		code, body := doJSON(t, client, http.MethodPost, "/pods", map[string]any{
			"name": name,
		})
		if code < 200 || code >= 300 {
			t.Fatalf("create+start pod: HTTP %d: %s", code, body)
		}
		code, body = doJSON(t, client, http.MethodGet, "/pods/"+name, nil)
		if code != 200 {
			t.Fatalf("inspect pod: HTTP %d: %s", code, body)
		}
		code, body = doJSON(t, client, http.MethodDelete, "/pods/"+name, nil)
		if code < 200 || code >= 300 {
			t.Fatalf("delete pod: HTTP %d: %s", code, body)
		}
	})

	t.Run("imageInspect", func(t *testing.T) {
		id := firstImageID(t, conn)
		if id == "" {
			t.Skip("no local images")
		}
		code, body := doJSON(t, client, http.MethodGet, "/images/"+id, nil)
		if code != 200 {
			t.Fatalf("inspect image %s: HTTP %d: %s", id, code, body)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(got["Id"]) == "" && fmt.Sprint(got["ID"]) == "" && fmt.Sprint(got["id"]) == "" {
			t.Fatalf("inspect missing id: %s", body)
		}
	})

	t.Run("container", func(t *testing.T) {
		image := firstImageRef(t, conn)
		if image == "" {
			t.Skip("no local images to create a container from")
		}
		name := "pt-ctr-" + suffix
		cleanup(t, client, http.MethodDelete, "/containers/"+name)
		code, body := doJSON(t, client, http.MethodPost, "/containers", map[string]any{
			"name":  name,
			"image": image,
		})
		if code < 200 || code >= 300 {
			t.Fatalf("create+start container: HTTP %d: %s", code, body)
		}
		var created map[string]any
		if err := json.Unmarshal(body, &created); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(created["Id"]) == "" && fmt.Sprint(created["ID"]) == "" {
			t.Fatalf("create container missing Id: %s", body)
		}
		code, body = doJSON(t, client, http.MethodGet, "/containers/"+name, nil)
		if code != 200 {
			t.Fatalf("inspect container: HTTP %d: %s", code, body)
		}
		var inspect map[string]any
		if err := json.Unmarshal(body, &inspect); err != nil {
			t.Fatal(err)
		}
		state, _ := inspect["State"].(map[string]any)
		status := strings.ToLower(fmt.Sprint(state["Status"]))
		if status == "created" || status == "" {
			t.Fatalf("start overlay did not start the container, State=%v", inspect["State"])
		}
		code, body = doJSON(t, client, http.MethodDelete, "/containers/"+name, nil)
		if code < 200 || code >= 300 {
			t.Fatalf("delete container: HTTP %d: %s", code, body)
		}
		var deleted map[string]any
		if err := json.Unmarshal(body, &deleted); err != nil {
			t.Fatalf("delete body should be coerced to an object, got %s: %v", body, err)
		}
	})
}

func pingEngine(conn transport.Conn) error {
	client := rawClient(conn)
	for _, path := range []string{"/v6.0.0/libpod/_ping", "/v5.0.0/libpod/_ping", "/libpod/_ping"} {
		req, err := http.NewRequest(http.MethodGet, "http://d"+path, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("podman engine not responding: %w", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
	}
	return fmt.Errorf("podman engine did not answer /libpod/_ping")
}

func rawClient(conn transport.Conn) *http.Client {
	dialer := net.Dialer{}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, conn.Network, conn.Address)
			},
			DisableCompression: true,
		},
	}
}

func firstImageID(t *testing.T, conn transport.Conn) string {
	t.Helper()
	images := listImages(t, conn)
	for _, img := range images {
		if id := strings.TrimPrefix(fmt.Sprint(img["Id"]), "sha256:"); id != "" && !strings.Contains(id, "/") {
			if i := strings.IndexByte(id, ':'); i >= 0 {
				id = id[:i]
			}
			if len(id) > 12 {
				return id[:12]
			}
			return id
		}
	}
	return ""
}

func firstImageRef(t *testing.T, conn transport.Conn) string {
	t.Helper()
	images := listImages(t, conn)
	for _, img := range images {
		names, _ := img["Names"].([]any)
		for _, n := range names {
			s := fmt.Sprint(n)
			if s != "" && s != "<none>:<none>" {
				return s
			}
		}
	}
	if id := firstImageID(t, conn); id != "" {
		return id
	}
	return ""
}

func listImages(t *testing.T, conn transport.Conn) []map[string]any {
	t.Helper()
	client := rawClient(conn)
	req, err := http.NewRequest(http.MethodGet, "http://d/v6.0.0/libpod/images/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Logf("list images: HTTP %d: %s", resp.StatusCode, raw)
		return nil
	}
	var images []map[string]any
	if err := json.Unmarshal(raw, &images); err != nil {
		t.Logf("list images: %v (%s)", err, raw)
		return nil
	}
	return images
}

func doJSON(t *testing.T, client *http.Client, method, path string, body any) (int, []byte) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	return doBytes(t, client, method, path, "application/json", raw)
}

func doBytes(t *testing.T, client *http.Client, method, path, contentType string, body []byte) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, "http://d"+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func cleanup(t *testing.T, client *http.Client, method, path string) {
	t.Helper()
	t.Cleanup(func() {
		req, err := http.NewRequest(method, "http://d"+path, nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	})
}
