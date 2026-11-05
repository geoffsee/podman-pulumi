// Package pconfig holds Pulumi provider configuration for Podman connections.
package pconfig

import (
	"context"
	"fmt"

	"github.com/geoffsee/pulumi-podman/internal/transport"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// Config is provider-level configuration for the Podman engine connection.
type Config struct {
	endpoint *transport.Endpoint `pulumi:"-"`

	ConnectionURI   string `pulumi:"connectionUri,optional"`
	SSHIdentityFile string `pulumi:"sshIdentityFile,optional"`
	SSHPrivateKey   string `pulumi:"sshPrivateKey,optional" provider:"secret"`
	SSHPassword     string `pulumi:"sshPassword,optional" provider:"secret"`
	SSHKnownHosts   string `pulumi:"sshKnownHosts,optional"`
}

// NewConfig returns infer config wired to the shared transport endpoint.
func NewConfig(endpoint *transport.Endpoint) infer.InferredConfig {
	return infer.Config(&Config{endpoint: endpoint})
}

// Annotate documents provider config fields.
func (c *Config) Annotate(a infer.Annotator) {
	a.Describe(&c.ConnectionURI, "Podman connection URI (unix://, tcp://, or ssh://). Overrides CONTAINER_HOST when set.")
	a.Describe(&c.SSHIdentityFile, "Path to the SSH private key for ssh:// connections. Also set via CONTAINER_SSHKEY.")
	a.Describe(&c.SSHPrivateKey, "SSH private key PEM for ssh:// connections. Prefer this over putting keys in stack outputs.")
	a.Describe(&c.SSHPassword, "SSH password when the server requires password authentication.")
	a.Describe(&c.SSHKnownHosts, "Path to an SSH known_hosts file. Defaults to ~/.ssh/known_hosts when present.")
}

// Configure opens or refreshes the Podman API connection.
func (c *Config) Configure(_ context.Context) error {
	if c.endpoint == nil {
		return fmt.Errorf("internal error: podman endpoint is not wired")
	}
	src := transport.Source{
		ConnectionURI:   c.ConnectionURI,
		SSHIdentityFile: c.SSHIdentityFile,
		SSHPrivateKey:   c.SSHPrivateKey,
		SSHPassword:     c.SSHPassword,
		SSHKnownHosts:   c.SSHKnownHosts,
	}
	return c.endpoint.Apply(src)
}
