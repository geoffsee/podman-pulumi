---
title: Podman
meta_desc: Use the Pulumi Podman provider to manage containers, pods, images, volumes, networks, secrets, and artifacts.
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

A local [Podman](https://podman.io) engine must be running and reachable. Point the provider at the API socket before `pulumi up`:

```bash
export CONTAINER_HOST=unix:///run/podman/podman.sock
```

On a rootless Linux user session the socket is typically `$XDG_RUNTIME_DIR/podman/podman.sock`. On macOS and Windows it is the [Podman machine](https://docs.podman.io/en/latest/markdown/podman-machine.1.html) socket. No `pulumi config set` values are required for a local engine.

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

The provider talks to the Podman API over a unix socket or TCP. Connection settings are read from the process environment rather than Pulumi config. A local engine needs none of the Pulumi config keys below.

| Name | Required? | Secret? | Description |
| --- | --- | --- | --- |
| `baseUrl` | no | no | Dummy HTTP origin used with the unix/TCP transport. Defaults to `http://d`. Leave unset. |
| `apiKey` | no | yes | Generic API key. Unused for a local Podman socket. |
| `apiKeyHeader` | no | no | Header name for `apiKey`. Unused for a local Podman socket. |
| `bearerToken` | no | yes | Bearer token for the `Authorization` header. Unused for a local Podman socket. |

Environment variables that select the engine (not Pulumi config keys):

| Variable | Required? | Description |
| --- | --- | --- |
| `CONTAINER_HOST` | no | Podman connection URI, for example `unix:///run/podman/podman.sock` or `tcp://127.0.0.1:8080`. Tried first. |
| `PODMAN_HOST` | no | Same as `CONTAINER_HOST`, used when `CONTAINER_HOST` is unset. |
| `PODMAN_API_VERSION` | no | Libpod API version prefix such as `5.0.0`. The provider otherwise uses the version baked from the swagger spec. Set this when the running engine is older than the spec. |

If neither host variable is set, the provider probes `$XDG_RUNTIME_DIR/podman/podman.sock`, then common rootful and `podman machine` socket paths.

Start the API if it is not already running:

```bash
podman system service --time=0
```

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
