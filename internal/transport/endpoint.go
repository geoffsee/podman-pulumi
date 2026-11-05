package transport

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Source describes where to find the Podman API endpoint.
type Source struct {
	ConnectionURI   string
	SSHIdentityFile string
	SSHPrivateKey   string
	SSHPassword     string
	SSHKnownHosts   string
}

// SourceFromEnv builds a Source from CONTAINER_HOST, PODMAN_HOST, and CONTAINER_SSHKEY.
func SourceFromEnv() Source {
	return Source{
		ConnectionURI:   firstNonEmpty(os.Getenv("CONTAINER_HOST"), os.Getenv("PODMAN_HOST")),
		SSHIdentityFile: os.Getenv("CONTAINER_SSHKEY"),
	}
}

// Endpoint holds the active dial target and optional SSH session cleanup.
type Endpoint struct {
	mu    sync.RWMutex
	dial  func(context.Context) (net.Conn, error)
	close func() error
}

// NewEndpoint returns an endpoint that must be configured with Apply before use.
func NewEndpoint() *Endpoint {
	return &Endpoint{}
}

// Dial opens a connection to the Podman API socket.
func (e *Endpoint) Dial(ctx context.Context) (net.Conn, error) {
	e.mu.RLock()
	dial := e.dial
	e.mu.RUnlock()
	if dial == nil {
		return nil, fmt.Errorf("podman endpoint is not configured")
	}
	return dial(ctx)
}

// Close releases any SSH session held by the endpoint.
func (e *Endpoint) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.close == nil {
		return nil
	}
	err := e.close()
	e.close = nil
	e.dial = nil
	return err
}

// Apply resolves the connection from src and env fallbacks, then updates the dialer.
func (e *Endpoint) Apply(src Source) error {
	if err := e.Close(); err != nil {
		return err
	}
	raw := strings.TrimSpace(src.ConnectionURI)
	if raw == "" {
		raw = firstNonEmpty(os.Getenv("CONTAINER_HOST"), os.Getenv("PODMAN_HOST"))
	}
	if raw == "" {
		conn, err := probeLocalSockets()
		if err != nil {
			return err
		}
		e.setDirect(conn)
		return nil
	}
	spec, err := ParseEndpointURI(raw)
	if err != nil {
		return err
	}
	if spec.Direct != nil {
		e.setDirect(*spec.Direct)
		return nil
	}
	tunnel, err := openSSHTunnel(*spec.SSH, src)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.dial = tunnel.dial
	e.close = tunnel.close
	e.mu.Unlock()
	return nil
}

func (e *Endpoint) setDirect(conn Conn) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	network, address := conn.Network, conn.Address
	e.mu.Lock()
	e.dial = func(ctx context.Context) (net.Conn, error) {
		return dialer.DialContext(ctx, network, address)
	}
	e.close = nil
	e.mu.Unlock()
}

func probeLocalSockets() (Conn, error) {
	var candidates []string
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "podman", "podman.sock"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock"),
			filepath.Join(home, ".config", "containers", "podman", "machine", "podman.sock"),
		)
	}
	candidates = append(candidates, "/run/podman/podman.sock", "/var/run/podman/podman.sock")
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return Conn{Network: "unix", Address: p}, nil
		}
	}
	return Conn{}, fmt.Errorf("no Podman socket found; set CONTAINER_HOST (for example unix:///run/user/%d/podman/podman.sock)", os.Getuid())
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
