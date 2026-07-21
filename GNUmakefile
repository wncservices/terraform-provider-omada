default: build

# Install dev tooling (docs generator + linter) into $(go env GOPATH)/bin.
# Versions track .tool-versions / go.mod.
GOLANGCI_LINT_VERSION ?= v2.12.2
tools:
	go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

# Build the provider binary into the module root.
build:
	go build -o terraform-provider-omada

# Install into the Go bin for local dev_overrides use.
install:
	go install

# Unit tests (fast, no controller required).
test:
	go test ./... -count=1

# Acceptance tests — drive the provider through a real Terraform binary against
# an in-process mock controller. Self-contained: no real hardware or secrets.
testacc:
	TF_ACC=1 go test ./... -v -count=1 -timeout 30m -run '^TestAcc'

lint:
	golangci-lint run

# Regenerate docs/ from schema + examples. Fails CI if the result differs.
docs:
	go generate ./...

fmt:
	gofmt -s -w -e .

.PHONY: tools build install test testacc lint docs fmt
