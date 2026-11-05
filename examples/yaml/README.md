# YAML example

Minimal program that creates a labeled volume against a **local** Podman engine.

## Prerequisites

- [Pulumi CLI](https://www.pulumi.com/docs/install/)
- Podman running with a reachable API socket (see [Configuration](../../README.md#configuration) in the root README)

## Run

From this directory, with the provider built at the repo root:

```sh
cd ../..
make build
cd examples/yaml
export CONTAINER_HOST=unix://$XDG_RUNTIME_DIR/podman/podman.sock   # adjust if needed
mkdir -p .pulumi-state
export PULUMI_BACKEND_URL="file://$(pwd)/.pulumi-state"
export PULUMI_CONFIG_PASSPHRASE=""
pulumi stack init dev --non-interactive 2>/dev/null || pulumi stack select dev
pulumi up --yes
pulumi destroy --yes
```

The `plugins.providers` entry points at the repository root so Pulumi loads your locally built `pulumi-resource-podman` binary.

## Remote engine (SSH)

Prefer Pulumi stack config for the connection and secrets (not stack outputs):

```yaml
# Pulumi.dev.yaml
config:
  podman:connectionUri: ssh://deploy@host.example/run/user/1000/podman/podman.sock
  podman:sshPrivateKey:
    secure: v1:...
```

Or set `CONTAINER_HOST` and `CONTAINER_SSHKEY` in the environment before `pulumi up`. See [Remote over SSH](../../README.md#remote-over-ssh) in the root README.
