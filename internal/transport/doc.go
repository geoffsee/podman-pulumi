// Package transport implements the HTTP client that dials the Podman Libpod API.
//
// Local connections use unix or TCP. Remote engines use ssh:// URIs: the provider
// opens an SSH session and forwards a channel to the remote podman.sock. Connection
// targets are selected via [Endpoint.Apply] from Pulumi provider config, environment
// variables (CONTAINER_HOST, PODMAN_HOST, CONTAINER_SSHKEY), or local socket probing.
package transport
