// Package openapibridge builds the OpenAPI dynamic provider with infer config support.
// pulumi-openapi-provider's ProviderBuilder does not expose WithConfig; this package
// duplicates the minimal wiring from that library.
package openapibridge

import (
	"fmt"

	openapi "github.com/pierskarsenbarg/pulumi-openapi-provider"
	"github.com/pierskarsenbarg/pulumi-openapi-provider/pkg/config"
	"github.com/pierskarsenbarg/pulumi-openapi-provider/pkg/runtime"
	"github.com/pierskarsenbarg/pulumi-openapi-provider/pkg/spec"
	p "github.com/pulumi/pulumi-go-provider"
)

// DynamicProvider returns the OpenAPI CRUD dispatch provider for opts.
func DynamicProvider(name, version string, opts openapi.Options) (p.Provider, error) {
	userAgent := config.ResolveUserAgent(opts.UserAgent, version)
	doc, err := spec.Load(opts.SpecURL, opts.SpecPath, opts.HTTPClient, userAgent)
	if err != nil {
		return p.Provider{}, fmt.Errorf("loading spec: %w", err)
	}

	overrides := convertOverrides(opts.Overrides)
	result, err := spec.Discover(doc, name, overrides, convertTypeOverrides(opts.TypeOverrides), opts.ExcludeTags)
	if err != nil {
		return p.Provider{}, fmt.Errorf("discovering resources: %w", err)
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = result.DefaultBaseURL
	}

	var authHeaderOverride string
	var tokenPrefixOverride *string
	if opts.AuthOverride != nil {
		authHeaderOverride = opts.AuthOverride.HeaderName
		tokenPrefixOverride = &opts.AuthOverride.TokenPrefix
	}
	cfg := config.New(opts.HTTPClient, baseURL, config.AuthSchemesFromSpec(result.AuthSchemes), authHeaderOverride, tokenPrefixOverride, userAgent)
	polling := runtime.ResolvePollingConfig(opts.PollingOptions.Timeout, opts.PollingOptions.InitialInterval,
		opts.PollingOptions.MaxInterval, opts.PollingOptions.Multiplier)
	hooks := buildHooks(result.Resources, opts.Overrides)
	return runtime.Build(name, version, result, cfg, !opts.DisablePolling, polling, hooks), nil
}

func buildHooks(resources []spec.ResourceDef, overrides map[string]openapi.ResourceOverride) map[string]runtime.ResourceHooks {
	if len(overrides) == 0 {
		return nil
	}
	wildcard := overrides["*"]
	hooks := make(map[string]runtime.ResourceHooks)
	for _, res := range resources {
		ov := overrides[res.Name]
		h := runtime.ResourceHooks{}
		if ov.Check != nil {
			h.Check = ov.Check
		} else {
			h.Check = wildcard.Check
		}
		if ov.Diff != nil {
			h.Diff = ov.Diff
		} else {
			h.Diff = wildcard.Diff
		}
		if ov.Create != nil {
			h.Create = ov.Create
		} else {
			h.Create = wildcard.Create
		}
		if ov.Read != nil {
			h.Read = ov.Read
		} else {
			h.Read = wildcard.Read
		}
		if ov.Update != nil {
			h.Update = ov.Update
		} else {
			h.Update = wildcard.Update
		}
		if ov.Delete != nil {
			h.Delete = ov.Delete
		} else {
			h.Delete = wildcard.Delete
		}
		if h.Check != nil || h.Diff != nil || h.Create != nil ||
			h.Read != nil || h.Update != nil || h.Delete != nil {
			hooks[res.Token] = h
		}
	}
	return hooks
}

func convertTypeOverrides(in map[string]openapi.TypeOverride) map[string]spec.TypeOverride {
	if in == nil {
		return nil
	}
	out := make(map[string]spec.TypeOverride, len(in))
	for k, v := range in {
		out[k] = spec.TypeOverride{Token: v.Token}
	}
	return out
}

func convertOverrides(in map[string]openapi.ResourceOverride) map[string]spec.ResourceOverride {
	if in == nil {
		return nil
	}
	out := make(map[string]spec.ResourceOverride, len(in))
	for k, v := range in {
		out[k] = spec.ResourceOverride{
			Skip:         v.Skip,
			Token:        v.Token,
			CreatePath:   v.CreatePath,
			CreateMethod: v.CreateMethod,
			ReadPath:     v.ReadPath,
			UpdatePath:   v.UpdatePath,
			UpdateMethod: v.UpdateMethod,
			DeletePath:   v.DeletePath,
			IDPathParam:  v.IDPathParam,
			IDField:      v.IDField,
		}
	}
	return out
}
