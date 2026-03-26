.PHONY: build docs test lint

BINARY := bin/nora

# build depends on docs so the spec is always fresh before compiling.
build: docs
	go build -o $(BINARY) ./cmd/nora

# docs generates the OpenAPI spec from swag annotations.
# Install swag CLI if not present: go install github.com/swaggo/swag/cmd/swag@v1.16.4
docs:
	@command -v swag > /dev/null 2>&1 || (echo "swag not found — run: go install github.com/swaggo/swag/cmd/swag@v1.16.4" && exit 1)
	swag init \
		-g main.go \
		-d cmd/nora,internal/api,internal/models \
		--parseInternal \
		-o docs/swagger

test:
	go test ./...

lint:
	golangci-lint run
