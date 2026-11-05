package pconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/geoffsee/pulumi-podman/internal/transport"
	p "github.com/pulumi/pulumi-go-provider"
)

// connectionConfigVariables are merged into GetSchema without infer.WithConfig so
// default-provider import stays stable (infer config changes default provider identity).
var connectionConfigVariables = map[string]any{
	"connectionUri": map[string]any{
		"type":        "string",
		"description": "Podman connection URI (unix://, tcp://, or ssh://). Overrides CONTAINER_HOST when set.",
	},
	"sshIdentityFile": map[string]any{
		"type":        "string",
		"description": "Path to the SSH private key for ssh:// connections. Also set via CONTAINER_SSHKEY.",
	},
	"sshPrivateKey": map[string]any{
		"type":        "string",
		"description": "SSH private key PEM for ssh:// connections. Prefer this over putting keys in stack outputs.",
		"secret":      true,
	},
	"sshPassword": map[string]any{
		"type":        "string",
		"description": "SSH password when the server requires password authentication.",
		"secret":      true,
	},
	"sshKnownHosts": map[string]any{
		"type":        "string",
		"description": "Path to an SSH known_hosts file. Defaults to ~/.ssh/known_hosts when present.",
	},
}

// Wrap applies connection Configure handling and schema variables to prov.
func Wrap(prov p.Provider, endpoint *transport.Endpoint) p.Provider {
	if endpoint == nil {
		return prov
	}
	prevGetSchema := prov.GetSchema
	prevConfigure := prov.Configure

	prov.GetSchema = func(ctx context.Context, req p.GetSchemaRequest) (p.GetSchemaResponse, error) {
		resp, err := prevGetSchema(ctx, req)
		if err != nil {
			return resp, err
		}
		schema, err := patchSchema(resp.Schema)
		if err != nil {
			return p.GetSchemaResponse{}, err
		}
		resp.Schema = schema
		return resp, nil
	}

	prov.Configure = func(ctx context.Context, req p.ConfigureRequest) error {
		if prevConfigure != nil {
			if err := prevConfigure(ctx, req); err != nil {
				return err
			}
		}
		return endpoint.Apply(SourceFromConfigure(req))
	}
	return prov
}

func patchSchema(raw string) (string, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", fmt.Errorf("parse provider schema: %w", err)
	}
	cfg, _ := doc["config"].(map[string]any)
	if cfg == nil {
		cfg = map[string]any{}
		doc["config"] = cfg
	}
	vars, _ := cfg["variables"].(map[string]any)
	if vars == nil {
		vars = map[string]any{}
		cfg["variables"] = vars
	}
	for k, v := range connectionConfigVariables {
		vars[k] = v
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal provider schema: %w", err)
	}
	return string(out), nil
}

// SourceFromConfigure reads Podman connection settings from a Configure request.
func SourceFromConfigure(req p.ConfigureRequest) transport.Source {
	src := transport.Source{
		ConnectionURI:   configString(req, "connectionUri"),
		SSHIdentityFile: configString(req, "sshIdentityFile"),
		SSHPrivateKey:   configString(req, "sshPrivateKey"),
		SSHPassword:     configString(req, "sshPassword"),
		SSHKnownHosts:   configString(req, "sshKnownHosts"),
	}
	return src
}

func configString(req p.ConfigureRequest, key string) string {
	if v, ok := req.Args.GetOk(key); ok && !v.IsNull() {
		return v.AsString()
	}
	if req.Variables != nil {
		return req.Variables[key]
	}
	return ""
}
