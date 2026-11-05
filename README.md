# Podman Provider for Pulumi

The Podman provider manages [Podman](https://podman.io) containers, pods, images, volumes, networks, secrets, and artifacts from a Pulumi program. It uses the Libpod REST API over a local unix/TCP socket or a remote engine reached through **SSH**.

## Installation

The plugin binary is installed from GitHub Releases. Language SDKs that GitHub Packages can host are published there.

* TypeScript: [`@geoffsee/podman`](https://github.com/geoffsee/podman-pulumi/pkgs/npm/podman) on `https://npm.pkg.github.com`
* .NET: [`Geoffsee.Podman`](https://github.com/geoffsee/podman-pulumi/pkgs/nuget/Geoffsee.Podman) on `https://nuget.pkg.github.com/geoffsee/index.json`
* Java: `com.geoffsee:podman` on `https://maven.pkg.github.com/geoffsee/podman-pulumi`

Python and Go have no GitHub Packages registry. Pulumi YAML does not use a language package. For those, generate the SDK from the schema:

```sh
pulumi package add podman
```

A GitHub token with `read:packages` is required to install the TypeScript, .NET, and Java packages. For npm:

```text
@geoffsee:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=TOKEN
```

## Configuration

Point the provider at a running Podman API. **No Pulumi config is required** for a default local engine.

### How the engine is chosen

On each provider `Configure` (including the first operation in a preview or update), the connection is resolved in this order:

1. **Pulumi provider config** — `connectionUri` and SSH settings (`sshIdentityFile`, `sshPrivateKey`, `sshPassword`, `sshKnownHosts`) when set on the stack or default provider.
2. **Environment** — `CONTAINER_HOST`, then `PODMAN_HOST`; for SSH, `CONTAINER_SSHKEY` supplies a key file path when `sshIdentityFile` is unset.
3. **Local socket probe** — `$XDG_RUNTIME_DIR/podman/podman.sock`, common rootful paths, and Podman machine sockets.

`connectionUri` in Pulumi config overrides `CONTAINER_HOST` / `PODMAN_HOST`. The OpenAPI-derived keys `baseUrl`, `apiKey`, `apiKeyHeader`, and `bearerToken` remain in the schema for compatibility; they are unused for normal Podman socket access (leave them unset).

Set `PODMAN_API_VERSION` (for example `5.0.0`) when the running engine is older than the API version baked into the provider.

### Local socket (default)

```sh
export CONTAINER_HOST=unix:///run/user/1000/podman/podman.sock
```

If neither host variable is set, the provider probes common local socket paths automatically.

### TCP

```sh
export CONTAINER_HOST=tcp://127.0.0.1:8080
```

Run `podman system service` (or equivalent) so the API listens on that address.

### Remote over SSH

Use an `ssh://` URI in the same shape as [`podman system connection`](https://docs.podman.io/en/latest/markdown/podman-system-connection.1.html):

```text
ssh://[user[:password]@]host[:port]/path/to/podman.sock
```

Defaults: SSH port **22**, remote socket **`/run/podman/podman.sock`** when the path is omitted, SSH user **`$USER`** when omitted from the URI.

**Authentication** (first method that succeeds):

- `sshPrivateKey` (Pulumi secret) or `sshIdentityFile` / `CONTAINER_SSHKEY` — private key file or PEM
- Password in the URI, or `sshPassword` in Pulumi config
- SSH agent (`SSH_AUTH_SOCK`)

**Host keys:** `sshKnownHosts` points at a `known_hosts` file; if unset, `~/.ssh/known_hosts` is used when present.

Supply credentials via **Pulumi provider config or environment variables**, not stack outputs:

```yaml
# Pulumi.dev.yaml
config:
  podman:connectionUri: ssh://deploy@host.example/run/user/1000/podman/podman.sock
  podman:sshPrivateKey:
    secure: PLAINTEXT
```

```sh
export CONTAINER_HOST='ssh://deploy@host.example/run/user/1000/podman/podman.sock'
export CONTAINER_SSHKEY="$HOME/.ssh/id_ed25519"
```

TypeScript (default provider):

```typescript
import * as pulumi from "@pulumi/pulumi";
import * as podman from "@geoffsee/podman";

const cfg = new pulumi.Config("podman");
const provider = new podman.Provider("remote", {
    connectionUri: cfg.require("connectionUri"),
    sshPrivateKey: cfg.requireSecret("sshPrivateKey"),
});

new podman.volumes.Volume("data", { name: "data" }, { provider });
```

### Provider config reference

| Key | Secret? | Description |
| --- | --- | --- |
| `connectionUri` | no | `unix://`, `tcp://`, or `ssh://` engine URI. Overrides `CONTAINER_HOST` / `PODMAN_HOST`. |
| `sshIdentityFile` | no | Path to SSH private key for `ssh://`. Also `CONTAINER_SSHKEY`. |
| `sshPrivateKey` | yes | SSH private key PEM for `ssh://`. |
| `sshPassword` | yes | SSH password when required. |
| `sshKnownHosts` | no | Path to SSH `known_hosts`. |
| `baseUrl` | no | OpenAPI placeholder; default `http://d`. Leave unset. |
| `apiKey` | yes | OpenAPI generic key; unused for Podman. |
| `apiKeyHeader` | no | OpenAPI key header; unused for Podman. |
| `bearerToken` | yes | OpenAPI bearer token; unused for Podman. |

### Environment variables

| Variable | Description |
| --- | --- |
| `CONTAINER_HOST` | Connection URI (`unix://`, `tcp://`, or `ssh://`). |
| `PODMAN_HOST` | Same as `CONTAINER_HOST` if `CONTAINER_HOST` is unset. |
| `CONTAINER_SSHKEY` | Path to SSH private key for `ssh://` when not using `sshIdentityFile` / `sshPrivateKey`. |
| `PODMAN_API_VERSION` | Libpod API version prefix (for example `5.0.0`). |
| `SSH_AUTH_SOCK` | Unix socket for ssh-agent (optional SSH auth). |

On macOS and Windows, use the [Podman machine](https://docs.podman.io/en/latest/markdown/podman-machine.1.html) socket or SSH to a Linux host. Start a local API if needed:

```sh
podman system service --time=0
```

## Example

```yaml
name: podman-example
runtime: yaml
resources:
  data:
    type: podman:volumes:Volume
    properties:
      name: data
outputs:
  volumeName: ${data.name}
```

```typescript
import * as podman from "@geoffsee/podman";

const data = new podman.volumes.Volume("data", {
    name: "data",
});

export const volumeName = data.name;
```

See [`examples/yaml`](examples/yaml) for a runnable YAML program and [`playground`](playground) for TypeScript (`make playground`).

## Resources

| Resource | Description |
| --- | --- |
| `podman:containers:Container` | Run a container. Creating it also starts it. Spec changes replace it. |
| `podman:pods:Pod` | Run a pod. Creating it also starts it. Spec changes replace it. |
| `podman:volumes:Volume` | Create a named volume. |
| `podman:networks:Network` | Create a network. |
| `podman:secrets:Secret` | Create a secret. |
| `podman:images:Image` | Pull an image. |
| `podman:artifacts:Artifact` | Pull an OCI artifact. |

Container and pod deletes pass `force=true`. Fields that only appear on inspect do not show up as drift.

## Importing existing resources

Adopt a resource that already exists on the engine (volume name, container ID, etc.—see the [Pulumi Registry](https://www.pulumi.com/registry/) import docs for each token).

1. Add a resource block to your program that matches the live object (name, labels, and other properties you intend to manage).
2. Import by type token, Pulumi resource name, and engine ID:

```sh
pulumi stack init dev
pulumi import podman:volumes:Volume data my-existing-volume
pulumi preview   # expect no changes when the program matches the engine
```

Example program:

```yaml
name: podman-import
runtime: yaml
resources:
  data:
    type: podman:volumes:Volume
    properties:
      name: my-existing-volume
      labels:
        app: demo
```

**Default provider:** You can import with the implicit default Podman provider; the program does not need a separate `pulumi:providers:podman` resource.

**Explicit provider:** If your program declares its own `pulumi:providers:podman` resource and attaches it with `options.provider`, create that provider in the stack first, then import with the same provider instance:

```sh
pulumi up --target 'urn:pulumi:STACK::PROJECT::pulumi:providers:podman::RESOURCE_NAME' --yes
pulumi import podman:volumes:Volume data my-existing-volume \
  --provider 'podman=urn:pulumi:STACK::PROJECT::pulumi:providers:podman::RESOURCE_NAME'
```

Replace `STACK`, `PROJECT`, and `RESOURCE_NAME` with your stack, project, and provider resource name.

## Development

```sh
make test
make build
make install
```

`make generate` refreshes the embedded Libpod spec. `make schema` regenerates `schema.json` from the provider binary. `make sdk` writes language SDKs under `sdk/`.

Package documentation for the [Pulumi Registry](https://www.pulumi.com/registry/) lives in [`docs/_index.md`](docs/_index.md).
