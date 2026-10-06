#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
version="$(tr -d ' \n' < VERSION)"
mkdir -p dist
rm -f dist/pulumi-resource-podman-v"${version}"-*.tar.gz dist/checksums.txt

for spec in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	os="${spec%/*}"
	arch="${spec#*/}"
	ext=""
	if [[ "$os" == windows ]]; then
		ext=".exe"
	fi
	out="dist/pulumi-resource-podman${ext}"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$out" .
	tar -C dist -czf "dist/pulumi-resource-podman-v${version}-${os}-${arch}.tar.gz" "pulumi-resource-podman${ext}"
	rm -f "$out"
done

(
	cd dist
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum pulumi-resource-podman-v"${version}"-*.tar.gz >checksums.txt
	else
		shasum -a 256 pulumi-resource-podman-v"${version}"-*.tar.gz >checksums.txt
	fi
)
echo "Wrote release archives for v${version} under dist/"
