---
title: Podman
meta_desc: Manage Podman containers, pods, images, volumes, networks, secrets, and artifacts with Pulumi—local sockets, TCP, or remote SSH.
layout: package
---

The Podman provider for Pulumi lets you manage [Podman](https://podman.io) resources, including containers, pods, images, volumes, networks, secrets, and artifacts, as part of your Pulumi programs. The resource surface is generated from the [Libpod REST API](https://docs.podman.io/en/latest/_static/api.html) so it tracks upstream Podman as the swagger spec changes.

## Installation

The plugin binary comes from GitHub Releases. Language SDKs that GitHub Packages can host are published there, not to npmjs.com, NuGet.org, Maven Central, or PyPI.

{{< chooser language "typescript,python,go,csharp,java,yaml" >}}
{{% choosable language typescript %}}

GitHub Packages npm registry. Add this to `.npmrc` (a GitHub token with `read:packages`):

```text
@geoffsee:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=TOKEN
```

```bash
npm install @geoffsee/podman
```

{{% /choosable %}}
{{% choosable language python %}}

GitHub Packages has no Python registry, so this SDK is not published. Generate it locally:

```bash
pulumi package add podman
```

{{% /choosable %}}
{{% choosable language go %}}

GitHub Packages has no Go module registry, so this SDK is not published. Generate it locally:

```bash
pulumi package add podman
```

{{% /choosable %}}
{{% choosable language csharp %}}

GitHub Packages NuGet feed. Add a `nuget.config` source `https://nuget.pkg.github.com/geoffsee/index.json` with a GitHub token, then:

```bash
dotnet add package Geoffsee.Podman
```

{{% /choosable %}}
{{% choosable language java %}}

GitHub Packages Maven registry `https://maven.pkg.github.com/geoffsee/pulumi-podman`:

```xml
<dependency>
  <groupId>com.geoffsee</groupId>
  <artifactId>podman</artifactId>
  <version>0.1.0</version>
</dependency>
```

{{% /choosable %}}
{{% choosable language yaml %}}

```bash
pulumi package add podman
```

{{% /choosable %}}
{{< /chooser >}}

## Example Usage

A [Podman](https://podman.io) engine must be running and reachable (local socket, TCP, or remote over SSH). For a local engine, point the provider at the API before `pulumi up`:

```bash
export CONTAINER_HOST=unix:///run/podman/podman.sock
```

On a rootless Linux user session the socket is typically `$XDG_RUNTIME_DIR/podman/podman.sock`. On macOS and Windows it is the [Podman machine](https://docs.podman.io/en/latest/markdown/podman-machine.1.html) socket. No `pulumi config set` values are required for a local engine. See [Configuration](#configuration) for remote `ssh://` URIs and stack secrets.

{{< chooser language "typescript,python,go,csharp,yaml" >}}
{{% choosable language typescript %}}

```typescript
import * as podman from "@geoffsee/podman";

const data = new podman.volumes.Volume("data", {
    name: "data",
});

export const volumeName = data.name;
```

{{% /choosable %}}
{{% choosable language python %}}

```python
import pulumi
import pulumi_podman as podman

data = podman.volumes.Volume("data", name="data")
pulumi.export("volume_name", data.name)
```

{{% /choosable %}}
{{% choosable language go %}}

```go
package main

import (
	"github.com/geoffsee/pulumi-podman/sdk/go/podman/volumes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		data, err := volumes.NewVolume(ctx, "data", &volumes.VolumeArgs{
			Name: pulumi.String("data"),
		})
		if err != nil {
			return err
		}
		ctx.Export("volumeName", data.Name)
		return nil
	})
}
```

{{% /choosable %}}
{{% choosable language csharp %}}

```csharp
using System.Collections.Generic;
using Pulumi;
using Podman = Geoffsee.Podman;

return await Deployment.RunAsync(() =>
{
    var data = new Podman.Volumes.Volume("data", new()
    {
        Name = "data",
    });

    return new Dictionary<string, object?>
    {
        ["volumeName"] = data.Name,
    };
});
```

{{% /choosable %}}
{{% choosable language yaml %}}

```yaml
name: podman-example
runtime: yaml
description: Create a Podman volume
resources:
  data:
    type: podman:volumes:Volume
    properties:
      name: data
outputs:
  volumeName: ${data.name}
```

{{% /choosable %}}
{{< /chooser >}}

## Configuration

The provider talks to the Podman API over a unix socket, TCP, or an SSH tunnel to a remote unix socket. A local engine needs no Pulumi config keys.

### Connection resolution order

1. Pulumi provider config — `connectionUri` and SSH keys on the default or explicit provider.
2. Environment — `CONTAINER_HOST`, then `PODMAN_HOST`; `CONTAINER_SSHKEY` for SSH key file path when `sshIdentityFile` is unset.
3. Local socket probe — `$XDG_RUNTIME_DIR/podman/podman.sock`, then common rootful and Podman machine paths.

`connectionUri` in stack config overrides `CONTAINER_HOST` / `PODMAN_HOST`.

### Pulumi provider config

| Name | Required? | Secret? | Description |
| --- | --- | --- | --- |
| `connectionUri` | no | no | `unix://`, `tcp://`, or `ssh://` URI. Overrides environment host variables. |
| `sshIdentityFile` | no | no | Path to SSH private key for `ssh://`. Same role as `CONTAINER_SSHKEY`. |
| `sshPrivateKey` | no | yes | SSH private key PEM for `ssh://`. Prefer stack secrets over outputs. |
| `sshPassword` | no | yes | SSH password when the server requires it. |
| `sshKnownHosts` | no | no | Path to SSH `known_hosts`. Defaults to `~/.ssh/known_hosts` when present. |
| `baseUrl` | no | no | OpenAPI transport placeholder (`http://d`). Leave unset. |
| `apiKey` | no | yes | OpenAPI generic key. Unused for Podman. |
| `apiKeyHeader` | no | no | OpenAPI key header name. Unused for Podman. |
| `bearerToken` | no | yes | OpenAPI bearer token. Unused for Podman. |

### Environment variables

| Variable | Required? | Description |
| --- | --- | --- |
| `CONTAINER_HOST` | no | Connection URI: `unix://…`, `tcp://…`, or `ssh://user@host/…/podman.sock`. |
| `PODMAN_HOST` | no | Same as `CONTAINER_HOST` when `CONTAINER_HOST` is unset. |
| `CONTAINER_SSHKEY` | no | Path to SSH private key for `ssh://` when not using Pulumi `sshIdentityFile` / `sshPrivateKey`. |
| `PODMAN_API_VERSION` | no | Libpod API version prefix (for example `5.0.0`) when the engine is older than the embedded spec. |
| `SSH_AUTH_SOCK` | no | Optional ssh-agent socket for SSH authentication. |

### Remote engines (`ssh://`)

URI form (same idea as `podman system connection`):

```text
ssh://[user[:password]@]host[:port]/path/to/podman.sock
```

Default SSH port is 22; default remote socket path is `/run/podman/podman.sock` when omitted. Authentication tries, in order: Pulumi `sshPrivateKey`, key file (`sshIdentityFile` or `CONTAINER_SSHKEY`), password in the URI or `sshPassword`, then keys from `SSH_AUTH_SOCK`.

Example stack config:

```yaml
config:
  podman:connectionUri: ssh://deploy@host.example/run/user/1000/podman/podman.sock
  podman:sshPrivateKey:
    secure: PLAINTEXT
```

Start a local API service if nothing is listening yet:

```bash
podman system service --time=0
```

## Importing existing resources

Use `pulumi import` to adopt an object that already exists on the engine. The import ID is resource-specific (for volumes, the volume name). Add a matching resource to your program first:

```yaml
resources:
  data:
    type: podman:volumes:Volume
    properties:
      name: my-volume
      labels:
        app: demo
```

Then import and verify:

```bash
pulumi import podman:volumes:Volume data my-volume
pulumi preview   # no changes when properties match the engine
```

Programs that rely on the **default** Podman provider do not need a separate `pulumi:providers:podman` resource.

If the program declares an **explicit** `pulumi:providers:podman` and sets `options.provider` on resources, create that provider in the stack (`pulumi up --target <provider-urn>`), then import with `--provider 'podman=<provider-urn>'` so preview does not replace the resource because of a provider reference mismatch.

## Resources

Typical resources (the exact set follows the current Libpod swagger):

| Token | Upstream |
| --- | --- |
| `podman:containers:Container` | `/libpod/containers/*` |
| `podman:pods:Pod` | `/libpod/pods/*` |
| `podman:volumes:Volume` | `/libpod/volumes/*` |
| `podman:networks:Network` | `/libpod/networks/*` |
| `podman:secrets:Secret` | `/libpod/secrets/*` |
| `podman:images:Image` | `/libpod/images/*` |
| `podman:artifacts:Artifact` | `/libpod/artifacts/*` |

Container and pod creates also start the resource after a successful create. Deletes on containers and pods pass `force=true`. Changing a container or pod spec replaces the resource. Inspect-only fields from GET do not count as drift.

## Prerequisites

- A running [Podman](https://podman.io) engine (Linux, or a Podman machine on macOS/Windows)
- [Pulumi](https://www.pulumi.com/docs/install/) 3.35.3 or later, so `pluginDownloadURL` of the form `github://api.github.com/...` resolves
