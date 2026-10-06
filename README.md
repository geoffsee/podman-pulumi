# pulumi-podman

A Pulumi provider for [Podman](https://podman.io), generated from the upstream [Libpod REST API](https://docs.podman.io/en/latest/_static/api.html).

The resource surface is not hand-written. `make generate` downloads the current Podman swagger spec, rewrites Libpod's `/create` and `/json` conventions into REST CRUD, and the [pulumi-openapi-provider](https://github.com/pierskarsenbarg/pulumi-openapi-provider) framework discovers resources from that document at runtime. When Podman adds fields or endpoints, regenerate.

## One command to stay in sync

```sh
make generate
```

That:

1. Fetches `https://docs.podman.io/en/latest/_static/swagger.yaml`
2. Keeps only `/libpod/*` operations (the native API, not the Docker-compat aliases)
3. Rewrites `POST /libpod/{resource}/create` → `POST /{resource}` and `GET /libpod/{resource}/{name}/json` → `GET /{resource}/{id}`
4. Writes the adapted spec and a path map used at runtime to hit the real Podman URLs

Pin a Podman release instead of latest:

```sh
make generate SPEC_URL=https://docs.podman.io/en/v6.0.0/_static/swagger.yaml
```

Re-adapt an already-downloaded `spec/swagger.yaml` without hitting the network:

```sh
make generate-local
```

## CI and releases

The package follows [Publishing to the Pulumi Registry](https://www.pulumi.com/docs/iac/guides/building-extending/packages/publishing-packages/): a committed `schema.json`, overview docs at `docs/_index.md`, a `vX.Y.Z` GitHub release, and plugin archives named `pulumi-resource-podman-vVERSION-OS-ARCH.tar.gz`.

GitHub cannot subscribe to `docs.podman.io`, so `.github/workflows/sync-spec.yml` polls the swagger URL every six hours (and on manual dispatch). When the document changes it:

1. Writes `spec/swagger.yaml` and runs `make generate-local`
2. Runs `go test ./...` (live Podman tests skip on GitHub-hosted runners)
3. Bumps the patch version in `VERSION` if that tag already exists
4. Regenerates `schema.json` (version in the schema matches the release tag)
5. Commits the spec and schema, tags `vX.Y.Z`, and publishes a GitHub Release with plugin archives
6. Generates language SDKs and publishes the formats GitHub Packages can host

Force a publish of the current tree from the Actions UI with **force**. Optional secret `SYNC_TOKEN` is a PAT used instead of `GITHUB_TOKEN` when the default branch is protected. SDK publish uses the workflow `GITHUB_TOKEN` (`packages: write`), not `SYNC_TOKEN`.

Nothing is published to npmjs.com, NuGet.org, Maven Central, PyPI, or Sonatype.

| SDK | GitHub Packages | Registry |
| --- | --- | --- |
| Node.js | yes | `https://npm.pkg.github.com` (`@geoffsee/podman`) |
| NuGet | yes | `https://nuget.pkg.github.com/geoffsee/index.json` (`Geoffsee.Podman`) |
| Maven | yes | `https://maven.pkg.github.com/geoffsee/pulumi-podman` (`com.geoffsee:podman`) |
| Python | no | GitHub Packages has no Python registry. `sdk/python` is generated only. |
| Go | no | GitHub Packages has no Go module registry. `sdk/go` is generated only. |
| YAML, HCL | no | Those languages read the schema, not a language package. |

Install a released package:

```sh
pulumi package add podman
```

The schema sets `pluginDownloadURL` to `github://api.github.com/geoffsee/pulumi-podman`, so `pulumi install` fetches the matching plugin binary from the GitHub release. `.github/workflows/ci.yml` runs tests, `make check-schema`, and generates the SDKs (Node build, NuGet pack, Python wheel) without publishing them.

### Listing on the Pulumi Registry

After `geoffsee/pulumi-podman` exists and has a `v`-prefixed GitHub release, open one pull request against [`pulumi/registry`](https://github.com/pulumi/registry) that adds this entry to [`community-packages/package-list.json`](https://github.com/pulumi/registry/blob/master/community-packages/package-list.json):

```json
{
  "repoSlug": "geoffsee/pulumi-podman",
  "schemaFile": "schema.json"
}
```

In the same PR, register the publisher in [`publisher-names.json`](https://github.com/pulumi/registry/blob/master/tools/resourcedocsgen/pkg/publishers/publisher-names.json):

```json
"geoffsee": "geoffsee"
```

Later versions need only a new GitHub release. A scheduled job in `pulumi/registry` picks them up twice a day.

## Build and use

```sh
make test
make build
make install
```

Point a program at the local plugin (after `make build`, or at the repo root so Pulumi can `go run` it):

```yaml
name: use-podman
runtime: yaml
plugins:
  providers:
    - name: podman
      path: ../podman-pulumi
resources:
  data:
    type: podman:volumes:Volume
    properties:
      name: data
```

Or generate typed SDKs locally (`pulumi package add` does this in a program directory):

```sh
make sdk
```

## Connection

The provider talks to the Podman API socket. It uses, in order:

1. `CONTAINER_HOST`
2. `PODMAN_HOST`
3. `$XDG_RUNTIME_DIR/podman/podman.sock` if it exists
4. Common rootful and `podman machine` socket paths

Examples:

```sh
export CONTAINER_HOST=unix:///run/user/1000/podman/podman.sock
export CONTAINER_HOST=unix://$HOME/.local/share/containers/podman/machine/podman.sock
export CONTAINER_HOST=tcp://127.0.0.1:8080
```

Libpod CRUD lives under `/v{major}.0.0/libpod/...`. `make generate` bakes that prefix from the swagger `info.version`. Override it when the engine is older than the spec:

```sh
export PODMAN_API_VERSION=5.0.0
```

Start the API if it is not already running:

```sh
podman system service --time=0
```

## What gets generated

Typical resources (exact set follows the spec):

| Token | Upstream |
| --- | --- |
| `podman:containers:Container` | `/libpod/containers/*` |
| `podman:pods:Pod` | `/libpod/pods/*` |
| `podman:volumes:Volume` | `/libpod/volumes/*` |
| `podman:networks:Network` | `/libpod/networks/*` |
| `podman:secrets:Secret` | `/libpod/secrets/*` |
| `podman:images:Image` | `/libpod/images/*` |
| `podman:artifacts:Artifact` | `/libpod/artifacts/*` |

Container and pod creates also start the resource after a successful create, matching `podman run` / `podman pod start`. Deletes on containers pass `force=true`. Changing a container or pod spec replaces the resource. Inspect-only fields from GET do not count as drift.

Input and output properties come from the swagger schemas (`SpecGenerator`, `VolumeCreateOptions`, `Network`, …), so new fields land when you regenerate.

## Layout

| Path | Role |
| --- | --- |
| `spec/swagger.yaml` | Upstream spec snapshot |
| `internal/specadapt` | Swagger → REST rewrite |
| `internal/specdata` | Embedded adapted spec + path map |
| `internal/transport` | Unix/TCP client, `/vX.0.0` prefix, and path rewrite |
| `cmd/adapt-spec` | `make generate` helper |
| `main.go` | Provider binary (`pulumi-resource-podman`) |
| `schema.json` | Pulumi package schema (Registry + `pulumi package add`) |
| `docs/_index.md` | Registry overview page |
| `docs/logo.svg` | Official Podman wordmark used as `logoUrl` |

Hand-written code is the adapter, the socket transport, and a small set of resource-token overrides. Everything else is the spec.
