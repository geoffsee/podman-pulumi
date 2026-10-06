# Podman Provider for Pulumi

The Podman provider manages [Podman](https://podman.io) containers, pods, images, volumes, networks, secrets, and artifacts from a Pulumi program. It talks to a local Podman engine over the Libpod API.

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

Point the provider at a running Podman API. No Pulumi config is required for a local engine.

The provider checks these in order:

1. `CONTAINER_HOST`
2. `PODMAN_HOST`
3. `$XDG_RUNTIME_DIR/podman/podman.sock`
4. Common rootful and Podman machine sockets

```sh
export CONTAINER_HOST=unix:///run/user/1000/podman/podman.sock
```

On macOS and Windows this is the [Podman machine](https://docs.podman.io/en/latest/markdown/podman-machine.1.html) socket. Start the API if it is not already listening:

```sh
podman system service --time=0
```

Set `PODMAN_API_VERSION` (for example `5.0.0`) when the running engine is older than the API version built into the provider.

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

See [`examples/yaml`](examples/yaml) for a program you can run against a local engine.

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

## Development

```sh
make test
make build
make install
```

`make generate` refreshes the embedded Libpod spec. `make sdk` writes the language SDKs under `sdk/`.
