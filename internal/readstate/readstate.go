// Package readstate normalizes Podman inspect JSON into create-spec shaped
// property maps and merges prior inputs so refresh does not plan spurious replaces.
package readstate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
)

// Client is the Podman HTTP client (transport to http://d/...). Set from main before serving.
var Client *http.Client

func containerRead(ctx context.Context, req p.ReadRequest) (p.ReadResponse, error) {
	id := resourceID(req)
	raw, err := getAdapted(ctx, "/containers/"+url.PathEscape(id))
	if err != nil {
		return p.ReadResponse{}, err
	}
	live := goMapToPropertyMap(normalizeContainerInspect(raw))
	return finishRead(req, live), nil
}

func podRead(ctx context.Context, req p.ReadRequest) (p.ReadResponse, error) {
	id := resourceID(req)
	raw, err := getAdapted(ctx, "/pods/"+url.PathEscape(id))
	if err != nil {
		return p.ReadResponse{}, err
	}
	live := goMapToPropertyMap(normalizePodInspect(raw))
	return finishRead(req, live), nil
}

func imageRead(ctx context.Context, req p.ReadRequest) (p.ReadResponse, error) {
	id := resourceID(req)
	raw, err := getAdapted(ctx, "/images/"+url.PathEscape(id))
	if err != nil {
		return p.ReadResponse{}, err
	}
	live := goMapToPropertyMap(normalizeImageInspect(raw))
	return finishRead(req, live), nil
}

func volumeRead(ctx context.Context, req p.ReadRequest) (p.ReadResponse, error) {
	id := resourceID(req)
	raw, err := getAdapted(ctx, "/volumes/"+url.PathEscape(id))
	if err != nil {
		return p.ReadResponse{}, err
	}
	live := goMapToPropertyMap(normalizeVolumeInspect(raw))
	return finishRead(req, live), nil
}

func networkRead(ctx context.Context, req p.ReadRequest) (p.ReadResponse, error) {
	id := resourceID(req)
	raw, err := getAdapted(ctx, "/networks/"+url.PathEscape(id))
	if err != nil {
		return p.ReadResponse{}, err
	}
	live := goMapToPropertyMap(normalizeNetworkInspect(raw))
	return finishRead(req, live), nil
}

// Hooks exposes Read handlers for specdata.Overrides.
func Hooks() map[string]func(context.Context, p.ReadRequest) (p.ReadResponse, error) {
	return map[string]func(context.Context, p.ReadRequest) (p.ReadResponse, error){
		"Containers": containerRead,
		"Pods":       podRead,
		"Images":     imageRead,
		"Volumes":    volumeRead,
		"Networks":   networkRead,
	}
}

func resourceID(req p.ReadRequest) string {
	if req.ID != "" {
		return req.ID
	}
	for _, key := range []string{"id", "Id", "ID", "Name", "name"} {
		if v, ok := req.Properties.GetOk(key); ok && v.IsString() {
			s := strings.TrimSpace(v.AsString())
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func getAdapted(ctx context.Context, path string) (map[string]any, error) {
	if Client == nil {
		return nil, fmt.Errorf("readstate: HTTP client not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://d"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode GET %s: %w", path, err)
	}
	return out, nil
}

func finishRead(req p.ReadRequest, live property.Map) p.ReadResponse {
	if live.Len() == 0 {
		return p.ReadResponse{ID: req.ID, Properties: req.Properties, Inputs: req.Inputs}
	}
	merged := mergeInputsIntoOutputs(req.Properties, live)
	merged = mergeInputsIntoOutputs(req.Inputs, merged)
	return p.ReadResponse{ID: req.ID, Properties: merged, Inputs: merged}
}

func mergeInputsIntoOutputs(inputs, outputs property.Map) property.Map {
	result := map[string]property.Value{}
	outputs.All(func(k string, v property.Value) bool {
		result[k] = v
		return true
	})
	inputs.All(func(k string, v property.Value) bool {
		if _, exists := result[k]; !exists {
			result[k] = v
		}
		return true
	})
	return property.NewMap(result)
}

func normalizeContainerInspect(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	out := map[string]any{}
	if v, ok := raw["Name"]; ok {
		out["name"] = v
	}
	if v, ok := raw["Id"]; ok {
		out["id"] = v
	}
	cfg, _ := raw["Config"].(map[string]any)
	if cfg != nil {
		if v, ok := cfg["Image"]; ok && fmt.Sprint(v) != "" {
			out["image"] = v
		}
		if v, ok := cfg["Cmd"]; ok {
			out["command"] = v
		}
		if v, ok := cfg["Entrypoint"]; ok {
			out["entrypoint"] = v
		}
		if v, ok := cfg["Labels"]; ok {
			out["labels"] = v
		}
		if v, ok := cfg["Env"]; ok {
			out["env"] = envSliceToMap(v)
		}
	}
	if v, ok := raw["ImageName"]; ok && out["image"] == nil {
		out["image"] = v
	}
	if ns, _ := raw["NetworkSettings"].(map[string]any); ns != nil {
		if nets, _ := ns["Networks"].(map[string]any); len(nets) > 0 {
			netOut := map[string]any{}
			for name := range nets {
				netOut[name] = map[string]any{}
			}
			out["networks"] = netOut
		}
	}
	if mounts, _ := raw["Mounts"].([]any); len(mounts) > 0 {
		var vols []any
		for _, m := range mounts {
			mount, _ := m.(map[string]any)
			if mount == nil {
				continue
			}
			if fmt.Sprint(mount["Type"]) != "volume" {
				continue
			}
			entry := map[string]any{}
			if name := mount["Name"]; fmt.Sprint(name) != "" {
				entry["name"] = name
			}
			if dest := mount["Destination"]; fmt.Sprint(dest) != "" {
				entry["dest"] = dest
			}
			if len(entry) > 0 {
				vols = append(vols, entry)
			}
		}
		if len(vols) > 0 {
			out["volumes"] = vols
		}
	}
	return out
}

func normalizePodInspect(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	out := map[string]any{}
	if v, ok := raw["Name"]; ok {
		out["name"] = v
	}
	if v, ok := raw["Id"]; ok {
		out["id"] = v
	}
	if v, ok := raw["Labels"]; ok {
		out["labels"] = v
	}
	return out
}

func normalizeImageInspect(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	out := map[string]any{}
	if v, ok := raw["Id"]; ok {
		out["id"] = v
	}
	if tags, ok := raw["RepoTags"].([]any); ok && len(tags) > 0 {
		out["reference"] = tags[0]
	}
	if tags, ok := raw["RepoTags"].([]any); ok {
		out["repoTags"] = tags
	}
	return out
}

func normalizeVolumeInspect(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	out := map[string]any{}
	if v, ok := raw["Name"]; ok {
		out["name"] = v
	}
	if v, ok := raw["Labels"]; ok {
		out["labels"] = v
	}
	return out
}

func normalizeNetworkInspect(raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	out := map[string]any{}
	if v, ok := raw["name"]; ok {
		out["name"] = v
	}
	if v, ok := raw["id"]; ok {
		out["id"] = v
	}
	if v, ok := raw["labels"]; ok {
		out["labels"] = v
	}
	return out
}

func envSliceToMap(v any) map[string]any {
	items, _ := v.([]any)
	out := map[string]any{}
	for _, item := range items {
		s, _ := item.(string)
		k, val, ok := strings.Cut(s, "=")
		if !ok || k == "" {
			continue
		}
		out[k] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func goMapToPropertyMap(m map[string]any) property.Map {
	if m == nil {
		return property.Map{}
	}
	vals := map[string]property.Value{}
	for k, v := range m {
		vals[k] = goValueToProperty(v)
	}
	return property.NewMap(vals)
}

func goValueToProperty(v any) property.Value {
	if v == nil {
		return property.New(property.Null)
	}
	switch tv := v.(type) {
	case bool:
		return property.New(tv)
	case float64:
		return property.New(tv)
	case string:
		return property.New(tv)
	case []any:
		items := make([]property.Value, len(tv))
		for i, item := range tv {
			items[i] = goValueToProperty(item)
		}
		return property.New(property.NewArray(items))
	case map[string]any:
		return property.New(goMapToPropertyMap(tv))
	default:
		return property.New(fmt.Sprint(tv))
	}
}
