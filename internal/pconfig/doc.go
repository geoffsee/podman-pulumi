// Package pconfig extends the OpenAPI provider with Podman connection settings.
//
// Connection keys (connectionUri, sshIdentityFile, sshPrivateKey, sshPassword,
// sshKnownHosts) are merged into the published schema and applied in Configure
// without infer.WithConfig, so default-provider import stays stable.
package pconfig
