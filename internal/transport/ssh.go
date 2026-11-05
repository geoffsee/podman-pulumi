package transport

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHSpec is a parsed ssh:// Podman connection URI.
type SSHSpec struct {
	User         string
	Password     string
	Host         string
	Port         string
	RemoteSocket string
}

// EndpointSpec is either a local unix/tcp connection or an SSH target.
type EndpointSpec struct {
	Direct *Conn
	SSH    *SSHSpec
}

// ParseEndpointURI parses a Podman connection URI (unix, tcp, http(s), or ssh).
func ParseEndpointURI(raw string) (EndpointSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return EndpointSpec{}, fmt.Errorf("empty connection URI")
	}
	if strings.HasPrefix(raw, "ssh://") {
		spec, err := parseSSHURI(raw)
		if err != nil {
			return EndpointSpec{}, err
		}
		return EndpointSpec{SSH: &spec}, nil
	}
	conn, err := ParseConn(raw)
	if err != nil {
		return EndpointSpec{}, err
	}
	return EndpointSpec{Direct: &conn}, nil
}

func parseSSHURI(raw string) (SSHSpec, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return SSHSpec{}, err
	}
	if u.Scheme != "ssh" {
		return SSHSpec{}, fmt.Errorf("expected ssh scheme")
	}
	host := u.Hostname()
	if host == "" {
		return SSHSpec{}, fmt.Errorf("ssh URI requires a host")
	}
	port := u.Port()
	if port == "" {
		port = "22"
	}
	user := u.User.Username()
	if user == "" {
		user = os.Getenv("USER")
	}
	var password string
	if u.User != nil {
		if p, ok := u.User.Password(); ok {
			password = p
		}
	}
	socket := u.Path
	if socket == "" {
		socket = "/run/podman/podman.sock"
	}
	return SSHSpec{
		User:         user,
		Password:     password,
		Host:         host,
		Port:         port,
		RemoteSocket: socket,
	}, nil
}

type sshTunnel struct {
	client *ssh.Client
	path   string
}

func (t *sshTunnel) dial(ctx context.Context) (net.Conn, error) {
	type res struct {
		conn net.Conn
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := t.client.Dial("unix", t.path)
		ch <- res{c, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.conn, r.err
	}
}

func (t *sshTunnel) close() error {
	return t.client.Close()
}

func openSSHTunnel(spec SSHSpec, src Source) (*sshTunnel, error) {
	auth, err := sshAuthMethods(src, spec)
	if err != nil {
		return nil, err
	}
	hostKey, err := sshHostKeyCallback(src.SSHKnownHosts)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(spec.Host, spec.Port)
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            spec.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         15 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return &sshTunnel{client: client, path: spec.RemoteSocket}, nil
}

func sshAuthMethods(src Source, spec SSHSpec) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	keyPEM := strings.TrimSpace(src.SSHPrivateKey)
	if keyPEM != "" {
		signer, err := ssh.ParsePrivateKey([]byte(keyPEM))
		if err != nil {
			return nil, fmt.Errorf("parse sshPrivateKey: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	keyFile := firstNonEmpty(src.SSHIdentityFile, os.Getenv("CONTAINER_SSHKEY"))
	if keyFile != "" {
		keyFile = expandHome(keyFile)
		raw, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("read ssh identity %s: %w", keyFile, err)
		}
		signer, err := ssh.ParsePrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("parse ssh identity %s: %w", keyFile, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if spec.Password != "" {
		methods = append(methods, ssh.Password(spec.Password))
	}
	if p := strings.TrimSpace(src.SSHPassword); p != "" {
		methods = append(methods, ssh.Password(p))
	}

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			ag := agent.NewClient(conn)
			methods = append(methods, ssh.PublicKeysCallback(ag.Signers))
		}
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH authentication configured; set sshIdentityFile, sshPrivateKey, CONTAINER_SSHKEY, SSH_AUTH_SOCK, or sshPassword")
	}
	return methods, nil
}

func sshHostKeyCallback(knownHostsPath string) (ssh.HostKeyCallback, error) {
	path := strings.TrimSpace(knownHostsPath)
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".ssh", "known_hosts")
		}
	}
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			return knownhosts.New(path)
		}
	}
	return ssh.InsecureIgnoreHostKey(), nil
}

func expandHome(path string) string {
	if path == "" || !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
