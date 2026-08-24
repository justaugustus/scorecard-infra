include Makefile.swagger

LDFLAGS=-w -extldflags

scorecard-webapp: main.go app/server/*.go ## Runs go build on repo
	# Run go build and generate scorecard-webapp executable
	CGO_ENABLED=0 go build -trimpath -a -tags netgo -ldflags '$(LDFLAGS)' -o scorecard-webapp

# The build context is the repository root, not this directory: go.mod lives
# there. Prefer `make api-docker` from the root, which is what CI runs.
scorecard-webapp-docker: scorecard-webapp Dockerfile
	DOCKER_BUILDKIT=1 docker build .. --file Dockerfile --tag scorecard-webapp

# Upstream had a `linter` target here invoking golangci-lint against a
# .golangci.yml in this directory. That config was not imported: golangci-lint
# reads one config, so a second one here would be silently ignored (design W7).
# Its rules were reconciled into the repository-root config; run `make lint`
# from the root.

swagger: openapi.yaml
	swagger validate openapi.yaml
	swagger generate server -f openapi.yaml -q -r COPYRIGHT.txt -t app/generated  -A scorecard --flag-strategy=pflag --keep-spec-order --exclude-main --default-produces application/json --additional-initialism=SSF
	swagger generate client -f openapi.yaml -q -r COPYRIGHT.txt -t app/generated --keep-spec-order --additional-initialism=SSF
	@echo "# This file is generated after swagger runs as part of the build; do not edit!" > Makefile.swagger
	@echo "SWAGGER_GEN=`find app/generated/client app/generated/models/ app/generated/restapi/ -iname '*.go' | grep -v 'configure_scorecard' | sort -d | tr '\n' ' ' | sed 's/ $$//'`" >> Makefile.swagger

clean:
	rm -rf $(SWAGGER_GEN)

