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

	conn := detectConn()

	opts := openapi.Options{
		SpecPath:       specFile,
		BaseURL:        "http://d",
		HTTPClient:     transport.New(conn, rules),
		UserAgent:      "pulumi-podman/" + providerVersion,
		DisablePolling: false,
		Overrides:      specdata.Overrides(),
	}

	builder, err := openapi.NewProviderBuilder(providerName, providerVersion, opts)
	if err != nil {
		return err
	}
	builder.
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

	return builder.Run(context.Background())
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

// detectConn returns the Podman socket, or a dummy unix path so schema
// generation works when no engine is running.
func detectConn() transport.Conn {
	conn, err := transport.DetectConn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		return transport.Conn{Network: "unix", Address: "/run/podman/podman.sock"}
	}
	return conn
}
