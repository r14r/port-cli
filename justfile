default:
    @just --list

# Verify Go and package-manager availability
doctor:
    go version
    command -v port
    command -v brew

# Format Go source files
fmt:
    go fmt ./...

# Run unit tests
test:
    go test ./...

# Static analysis
lint:
    go vet ./...

# Build executable for the current host
build:
    mkdir -p dist
    CGO_ENABLED=0 go build -trimpath -o dist/ports-cli .

# Build macOS Intel and Apple Silicon binaries
build-macos:
    mkdir -p dist
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -o dist/ports-cli-darwin-amd64 .
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o dist/ports-cli-darwin-arm64 .

# Install executable in /usr/local/bin
install: build
    sudo install -m 755 dist/ports-cli /usr/local/bin/ports-cli

# Run local verification
ci: test lint build build-macos

# Update MacPorts
update: build
    ./dist/ports-cli update

# Preview Homebrew migration
migrate-dry-run: build
    ./dist/ports-cli migrate --dry-run

# Apply safe migration
migrate: build
    ./dist/ports-cli migrate --yes

# Display project version
version:
    @cat VERSION

# Remove generated files
clean:
    rm -rf dist
