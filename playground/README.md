# TypeScript playground

Run the generated [`@geoffsee/podman`](../../sdk/nodejs) SDK against a locally built provider binary.

## Prerequisites

- [Pulumi CLI](https://www.pulumi.com/docs/install/)
- Node.js 20+
- A reachable Podman API (local socket, TCP, or SSH — see [Configuration](../README.md#configuration) in the root README)

Generate and build the Node.js SDK once (or after schema changes):

```sh
make sdk
cd sdk/nodejs && npm install && npm run build
```

## Commands

From the repository root:

```sh
make playground
```

Or manually:

```sh
make build
cd playground
npm install
npm run build
mkdir -p .pulumi-state
export PULUMI_BACKEND_URL="file://$(pwd)/.pulumi-state"
export PULUMI_CONFIG_PASSPHRASE=""
pulumi stack select dev --non-interactive 2>/dev/null || pulumi stack init dev --non-interactive
pulumi preview --stack dev --diff
pulumi up --stack dev --yes
pulumi destroy --stack dev --yes
```

The program pulls `busybox:latest`, creates a labeled volume and network, and starts three long-running Busybox containers on that network (a ticker, a volume-backed logger, and a sleeper).
