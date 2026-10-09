SHELL := powershell.exe
.SHELLFLAGS := -NoProfile -Command

.PHONY: lint test build

lint:
	go vet ./...

test: export CGO_ENABLED = 1
test:
	go test -race '-coverprofile=coverage.out' ./...

build: export CGO_ENABLED = 0
build:
	go build -ldflags "-H=windowsgui -s -w" -o FancyBorderless.exe .
