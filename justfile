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
    CGO_ENABLED=0 go build -trimpath -o dist/port-cli .

# Build macOS Intel and Apple Silicon binaries
build-macos:
    mkdir -p dist
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -o dist/port-cli-darwin-amd64 .
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o dist/port-cli-darwin-arm64 .

# Install executable in /usr/local/bin
install: build
    sudo install -m 755 dist/port-cli /usr/local/bin/port-cli

# Run local verification
ci: test lint build build-macos

# Update MacPorts
update: build
    ./dist/port-cli update

# Update MacPorts silently with timestamped logs
update-crontab: build
    ./dist/port-cli update --crontab

# Preview Homebrew migration
migrate-dry-run: build
    ./dist/port-cli migrate --dry-run

# Select packages interactively in the terminal
migrate-ui: build
    ./dist/port-cli migrate --ui

# Select packages with detailed scan progress
migrate-ui-details: build
    ./dist/port-cli migrate --ui --details

# Apply safe migration
migrate: build
    ./dist/port-cli migrate --yes

# Display project version
version:
    @cat VERSION

# Remove generated files
clean:
    rm -rf dist
