package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	openapi "github.com/pierskarsenbarg/pulumi-openapi-provider"
	"github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"

	"github.com/geoffsee/pulumi-podman/internal/openapibridge"
	"github.com/geoffsee/pulumi-podman/internal/pconfig"
	"github.com/geoffsee/pulumi-podman/internal/readstate"
	"github.com/geoffsee/pulumi-podman/internal/specadapt"
	"github.com/geoffsee/pulumi-podman/internal/specdata"
	"github.com/geoffsee/pulumi-podman/internal/transport"
)

const (
	providerName      = "podman"
	pluginDownloadURL = "github://api.github.com/geoffsee/pulumi-podman"
	repositoryURL     = "https://github.com/geoffsee/pulumi-podman"
	logoURL           = "https://raw.githubusercontent.com/geoffsee/pulumi-podman/main/docs/logo.svg"
)

//go:embed VERSION
var versionFile string

var providerVersion = strings.TrimSpace(versionFile)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	specFile, cleanup, err := writeTempSpec()
	if err != nil {
		return err
	}
	defer cleanup()

	var rules []specadapt.Rule
	if err := json.Unmarshal(specdata.PathMap, &rules); err != nil {
		return fmt.Errorf("parse path map: %w", err)
	}

	endpoint := transport.NewEndpoint()
	if err := bootstrapEndpoint(endpoint); err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	defer func() { _ = endpoint.Close() }()

	client := transport.New(endpoint, rules)
	readstate.Client = client

	opts := openapi.Options{
		SpecPath:       specFile,
		BaseURL:        "http://d",
		HTTPClient:     client,
		UserAgent:      "pulumi-podman/" + providerVersion,
		DisablePolling: false,
		Overrides:      specdata.Overrides(),
	}

	dyn, err := openapibridge.DynamicProvider(providerName, providerVersion, opts)
	if err != nil {
		return err
	}

	builder := infer.NewProviderBuilder().
		WithWrapped(dyn).
		WithDisplayName("Podman").
		WithDescription("The Pulumi Podman provider manages containers, pods, images, volumes, networks, secrets, and artifacts through the Libpod REST API.").
		WithHomepage(repositoryURL).
		WithRepository(repositoryURL).
		WithPublisher("geoffsee").
		WithLicense("Apache-2.0").
		WithNamespace("geoffsee").
		WithLogoURL(logoURL).
		WithPluginDownloadURL(pluginDownloadURL).
		WithKeywords(
			"pulumi",
			"podman",
			"containers",
			"category/infrastructure",
			"kind/native",
		).
		WithLanguageMap(map[string]any{
			"nodejs": map[string]any{
				"packageName":          "@geoffsee/podman",
				"respectSchemaVersion": true,
			},
			"python": map[string]any{
				"packageName":          "pulumi_podman",
				"respectSchemaVersion": true,
				"pyproject":            map[string]any{"enabled": true},
			},
			"go": map[string]any{
				"importBasePath":                 "github.com/geoffsee/pulumi-podman/sdk/go/podman",
				"generateResourceContainerTypes": true,
				"respectSchemaVersion":           true,
			},
			"csharp": map[string]any{
				"rootNamespace":        "Geoffsee",
				"respectSchemaVersion": true,
			},
			"java": map[string]any{
				"basePackage": "com.geoffsee",
			},
		})

	prov := pconfig.Wrap(infer.Provider(builder.BuildOptions()), endpoint)
	return provider.RunProvider(context.Background(), providerName, providerVersion, prov)
}

func writeTempSpec() (string, func(), error) {
	dir, err := os.MkdirTemp("", "pulumi-podman-spec-*")
	if err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, "openapi.yaml")
	if err := os.WriteFile(path, specdata.AdaptedSpec, 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func bootstrapEndpoint(endpoint *transport.Endpoint) error {
	src := transport.SourceFromEnv()
	if err := endpoint.Apply(src); err != nil {
		endpoint.Apply(transport.Source{ConnectionURI: "unix:///run/podman/podman.sock"})
		return err
	}
	return nil
}
