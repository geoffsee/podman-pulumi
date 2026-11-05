#!/usr/bin/env bash
# Publish the Podman provider and the SDKs Grove can host.
# npm, PyPI, Maven, and the plugin archives go to GROVE_URL (default https://grove.gsio.dev).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if [[ -z "${GROVE_API_KEY:-}" ]]; then
	echo "GROVE_API_KEY is required" >&2
	exit 1
fi

grove_url="${GROVE_URL:-https://grove.gsio.dev}"
grove_url="${grove_url%/}"
case "$grove_url" in
https://*) ;;
*)
	echo "GROVE_URL must be https: ${grove_url}" >&2
	exit 1
	;;
esac

case "$grove_url" in
*npmjs.com* | *registry.npmjs.org* | *pypi.org* | *upload.pypi.org* | *nuget.org* | *api.nuget.org* | *oss.sonatype.org* | *central.sonatype.com* | *repo.maven.apache.org* | *maven-central.storage-download.googleapis.com*)
	echo "refusing public registry: ${grove_url}" >&2
	exit 1
	;;
esac

host="${grove_url#https://}"
version="$(tr -d '[:space:]' < VERSION)"
if [[ -z "$version" ]]; then
	echo "VERSION is empty" >&2
	exit 1
fi

if [[ ${#GROVE_API_KEY} -lt 8 || ${GROVE_API_KEY} != grv_* ]]; then
	echo "GROVE_API_KEY must be a Grove API token (grv_...)" >&2
	exit 1
fi

echo "Publishing podman ${version} to ${grove_url}"

# Confirm the token is accepted before the long build. A bad document is 400; a rejected token is 401.
auth_status="$(
	curl -sS -o /dev/null -w '%{http_code}' \
		-X PUT \
		-H "Authorization: Bearer ${GROVE_API_KEY}" \
		-H "Content-Type: application/json" \
		--data '{}' \
		"${grove_url}/npm/@geoffsee%2fpodman"
)"
echo "Grove token probe status: ${auth_status}"
if [[ "$auth_status" != "400" && "$auth_status" != "409" ]]; then
	echo "Grove rejected GROVE_API_KEY (expected 400 from an empty publish body)" >&2
	exit 1
fi

bash scripts/dist.sh
bash scripts/gen-sdk.sh

(
	cd sdk/nodejs
	npm config set "@geoffsee:registry" "${grove_url}/npm/"
	npm config set "//${host}/npm/:_authToken" "$GROVE_API_KEY"
	npm pkg delete publishConfig.registry
	npm install
	npm run build
	npm publish --registry "${grove_url}/npm/"
)

(
	cd sdk/python
	python -m pip install --disable-pip-version-check build twine
	python -m build
	twine upload \
		--repository-url "${grove_url}/pypi/legacy" \
		--username grove \
		--password "$GROVE_API_KEY" \
		--non-interactive \
		dist/*
)

init="$(mktemp)"
cat >"$init" <<'EOF'
gradle.projectsLoaded {
    rootProject.allprojects {
        afterEvaluate { project ->
            if (project.plugins.hasPlugin("maven-publish")) {
                project.publishing.repositories.maven {
                    name = "Grove"
                    url = uri(System.getenv("GROVE_MAVEN_URL"))
                    credentials {
                        username = "grove"
                        password = System.getenv("GROVE_API_KEY")
                    }
                }
            }
        }
    }
}
EOF
export PACKAGE_VERSION="$version"
export GROVE_MAVEN_URL="${grove_url}/maven"
export GROVE_API_KEY
gradle -p sdk/java publishMainPublicationToGroveRepository --init-script "$init" --no-daemon
rm -f "$init"

echo "$GROVE_API_KEY" | oras login "$host" --username grove --password-stdin
files=()
for archive in dist/pulumi-resource-podman-v"${version}"-*.tar.gz; do
	files+=("$archive")
done
files+=(dist/checksums.txt)
oras push "${host}/geoffsee/pulumi-podman:${version}" \
	--annotation "org.opencontainers.image.source=https://github.com/geoffsee/podman-pulumi" \
	--annotation "org.opencontainers.image.version=${version}" \
	--annotation "org.opencontainers.image.title=pulumi-podman" \
	"${files[@]}"

echo "Published podman ${version} to ${grove_url}"
echo "  npm   @geoffsee/podman"
echo "  pypi  pulumi-podman"
echo "  maven com.geoffsee:podman"
echo "  oci   ${host}/geoffsee/pulumi-podman:${version}"
