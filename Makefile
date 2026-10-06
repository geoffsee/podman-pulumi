SPEC_URL ?= https://docs.podman.io/en/latest/_static/swagger.yaml
VERSION  ?= $(shell tr -d ' \n' < VERSION)
PROVIDER := bin/pulumi-resource-podman
SCHEMA   := schema.json

.PHONY: generate generate-local build dist test schema check-schema sdk publish-sdks install example

generate:
	mkdir -p spec internal/specdata
	curl -fsSL -o spec/swagger.yaml "$(SPEC_URL)"
	$(MAKE) generate-local
	@echo "Regenerated from $(SPEC_URL)"

generate-local:
	go run ./cmd/adapt-spec \
		-in spec/swagger.yaml \
		-out internal/specdata/openapi.adapted.yaml \
		-map internal/specdata/pathmap.json

build:
	go build -o $(PROVIDER) .

dist:
	bash scripts/dist.sh

test:
	go test ./...

# Isolated file backend so get-schema / schema check do not need a logged-in Pulumi account.
PULUMI_SCHEMA_ENV = PULUMI_SKIP_UPDATE_CHECK=true PULUMI_CONFIG_PASSPHRASE=

schema: build
	@tmpdir=$$(mktemp -d) && \
	mkdir -p "$$tmpdir/state" && \
	$(PULUMI_SCHEMA_ENV) PULUMI_BACKEND_URL="file://$$tmpdir/state" \
		pulumi package get-schema ./$(PROVIDER) > "$$tmpdir/schema.json" && \
	$(PULUMI_SCHEMA_ENV) PULUMI_BACKEND_URL="file://$$tmpdir/state" \
		pulumi schema check "$$tmpdir/schema.json" && \
	mv "$$tmpdir/schema.json" $(SCHEMA) && \
	rm -rf "$$tmpdir"
	@echo "Wrote $(SCHEMA)"

check-schema: schema
	@git ls-files --error-unmatch $(SCHEMA) >/dev/null
	@git diff --exit-code -- $(SCHEMA)

sdk:
	bash scripts/gen-sdk.sh

publish-sdks: sdk
	bash scripts/publish-sdks.sh

install: build
	mkdir -p "$${HOME}/.pulumi/plugins/resource-podman-v$(VERSION)"
	cp $(PROVIDER) "$${HOME}/.pulumi/plugins/resource-podman-v$(VERSION)/"
	@echo "Installed pulumi-resource-podman v$(VERSION)"

example: build
	mkdir -p examples/yaml/.pulumi-state
	cd examples/yaml && \
		PULUMI_BACKEND_URL="file://$$PWD/.pulumi-state" \
		PULUMI_CONFIG_PASSPHRASE="$${PULUMI_CONFIG_PASSPHRASE-}" \
		sh -c 'pulumi stack select dev --non-interactive 2>/dev/null || pulumi stack init dev --non-interactive'
	cd examples/yaml && \
		PULUMI_BACKEND_URL="file://$$PWD/.pulumi-state" \
		PULUMI_CONFIG_PASSPHRASE="$${PULUMI_CONFIG_PASSPHRASE-}" \
		pulumi preview --non-interactive --stack dev --diff
