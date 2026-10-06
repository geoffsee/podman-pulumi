#!/usr/bin/env bash
# Generate Pulumi SDKs from schema.json and point the npm/Maven manifests at
# GitHub Packages. Python and Go are generated too, but GitHub Packages has no
# registry for those formats.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

if [[ ! -f schema.json ]]; then
	make schema
fi

state="$(mktemp -d)"
mkdir -p "$state/state"
cleanup() { rm -rf "$state"; }
trap cleanup EXIT

rm -rf sdk
PULUMI_SKIP_UPDATE_CHECK=true \
PULUMI_CONFIG_PASSPHRASE= \
PULUMI_BACKEND_URL="file://${state}/state" \
	pulumi package gen-sdk ./schema.json --language all --out sdk

python3 - <<'PY'
import json
from pathlib import Path

pkg = json.loads(Path("sdk/nodejs/package.json").read_text())
pkg["main"] = "bin/index.js"
pkg["types"] = "bin/index.d.ts"
pkg["files"] = ["bin", "README.md"]
pkg["publishConfig"] = {"registry": "https://npm.pkg.github.com"}
repo = pkg.get("repository")
if isinstance(repo, str):
    pkg["repository"] = {"type": "git", "url": repo if repo.endswith(".git") else repo + ".git"}
Path("sdk/nodejs/package.json").write_text(json.dumps(pkg, indent=4) + "\n")
Path("sdk/nodejs/.npmignore").write_text(
    "node_modules/\n*.ts\n!bin/**/*.d.ts\ntsconfig.json\n.gitattributes\n.gitignore\n"
)

gradle = Path("sdk/java/build.gradle").read_text()
gradle = (
    gradle.replace("-Xmx16g", "-Xmx3g")
    .replace("-Xmx8g", "-Xmx2g")
    .replace("JavaLanguageVersion.of(11)", "JavaLanguageVersion.of(17)")
)
Path("sdk/java/build.gradle").write_text(gradle)
Path("sdk/java/github-packages.gradle").write_text(
    """// Applied after generation. Publishes only to GitHub Packages.
// Do not set PUBLISH_REPO_USERNAME: that enables the Sonatype/Maven Central plugin.
def ghRepo = System.getenv("GITHUB_REPOSITORY") ?: "geoffsee/pulumi-podman"
publishing {
    repositories {
        maven {
            name = "GitHubPackages"
            url = uri(System.getenv("GITHUB_PACKAGES_MAVEN_URL") ?: "https://maven.pkg.github.com/${ghRepo}")
            credentials {
                username = System.getenv("GITHUB_ACTOR") ?: ""
                password = System.getenv("GITHUB_TOKEN") ?: ""
            }
        }
    }
}
"""
)

Path("sdk/python/README.md").write_text(
    "The Pulumi Podman provider manages containers, pods, images, volumes, networks, secrets, and artifacts through the Libpod REST API.\n"
    "\nGitHub Packages has no Python registry. This SDK is generated for local use and is not published to PyPI.\n"
)

go_mod = Path("sdk/go/podman/go.mod")
go_mod.write_text(
    """module github.com/geoffsee/pulumi-podman/sdk/go/podman

go 1.22.0

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/pulumi/pulumi/sdk/v3 v3.142.0
)
"""
)
PY

# Make the extra Gradle script part of the generated build.
if ! grep -q 'github-packages.gradle' sdk/java/build.gradle; then
	printf '\napply from: "github-packages.gradle"\n' >> sdk/java/build.gradle
fi

cat <<'EOF'
Generated SDKs under sdk/.

GitHub Packages can host:
  nodejs  npm    @geoffsee/podman     https://npm.pkg.github.com
  dotnet  NuGet  Geoffsee.Podman      https://nuget.pkg.github.com/<owner>/index.json
  java    Maven  com.geoffsee:podman  https://maven.pkg.github.com/<owner>/<repo>

GitHub Packages cannot host:
  python  No Python/PyPI registry. sdk/python is generated only.
  go      No Go module registry. sdk/go is generated only; Go modules come from git.
  yaml    Pulumi YAML reads the schema, not a language package.
  hcl     Pulumi HCL reads the schema, not a language package.
EOF
