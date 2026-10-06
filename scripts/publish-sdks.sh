#!/usr/bin/env bash
# Publish generated Pulumi SDKs to GitHub Packages only.
# Never publishes to npmjs.com, NuGet.org, Maven Central, PyPI, or Sonatype.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
	echo "GITHUB_TOKEN is required to publish to GitHub Packages" >&2
	exit 1
fi
if [[ -z "${GITHUB_REPOSITORY:-}" ]]; then
	echo "GITHUB_REPOSITORY is required (owner/repo)" >&2
	exit 1
fi

owner="${GITHUB_REPOSITORY%%/*}"
# GitHub Packages npm scopes and Maven owner paths are lowercase.
owner_lc="$(printf '%s' "$owner" | tr '[:upper:]' '[:lower:]')"
actor="${GITHUB_ACTOR:-$owner}"
if [[ "$owner_lc" != "geoffsee" ]]; then
	echo "SDK package names are scoped to geoffsee (@geoffsee/podman, com.geoffsee:podman)." >&2
	echo "Refusing to publish from ${GITHUB_REPOSITORY}." >&2
	exit 1
fi

npm_registry="https://npm.pkg.github.com"
nuget_source="https://nuget.pkg.github.com/${owner_lc}/index.json"
maven_url="https://maven.pkg.github.com/${owner_lc}/${GITHUB_REPOSITORY#*/}"

refuse_external() {
	local url="$1"
	case "$url" in
	*npmjs.com* | *registry.npmjs.org* | *pypi.org* | *upload.pypi.org* | *nuget.org* | *api.nuget.org* | *oss.sonatype.org* | *central.sonatype.com* | *repo.maven.apache.org* | *maven-central.storage-download.googleapis.com*)
		echo "refusing external registry: ${url}" >&2
		exit 1
		;;
	esac
}
refuse_external "$npm_registry"
refuse_external "$nuget_source"
refuse_external "$maven_url"

if [[ ! -f sdk/nodejs/package.json ]]; then
	bash scripts/gen-sdk.sh
fi

echo "Publishing to GitHub Packages only."
echo "  npm   ${npm_registry}  scope @${owner_lc}"
echo "  nuget ${nuget_source}"
echo "  maven ${maven_url}"
echo "Skipping python: GitHub Packages has no Python registry."
echo "Skipping go: GitHub Packages has no Go module registry."

# Node.js → GitHub npm registry.
(
	cd sdk/nodejs
	cat >.npmrc <<EOF
@${owner_lc}:registry=${npm_registry}
//npm.pkg.github.com/:_authToken=\${NODE_AUTH_TOKEN}
EOF
	npm install
	npm run build
	NODE_AUTH_TOKEN="$GITHUB_TOKEN" npm publish --registry "$npm_registry"
)

# .NET → GitHub NuGet registry. Pack does not contact nuget.org.
dotnet pack sdk/dotnet/Geoffsee.Podman.csproj --configuration Release --output dist/nuget
dotnet nuget remove source github >/dev/null 2>&1 || true
dotnet nuget add source "$nuget_source" \
	--name github \
	--username "$actor" \
	--password "$GITHUB_TOKEN" \
	--store-password-in-clear-text
dotnet nuget push dist/nuget/*.nupkg \
	--api-key "$GITHUB_TOKEN" \
	--source github \
	--skip-duplicate

# Java → GitHub Maven registry. PUBLISH_REPO_USERNAME stays unset so the
# generated Sonatype/Maven Central plugin does not run.
export GITHUB_ACTOR="$actor"
export GITHUB_TOKEN
export GITHUB_REPOSITORY
export GITHUB_PACKAGES_MAVEN_URL="$maven_url"
gradle -p sdk/java publish --no-daemon

echo "Published npm, NuGet, and Maven SDKs to GitHub Packages."
echo "Not published: python (no GitHub Packages registry), go (no GitHub Packages registry)."
