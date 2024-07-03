# vim: ts=4 sw=4 ft=make

MAKEFILE_PATH := $(realpath $(lastword $(MAKEFILE_LIST)))
MAKEFILE_DIR  := $(abspath $(dir $(MAKEFILE_PATH)))

GOBIN = $(MAKEFILE_DIR)/bin

.PHONY: all
all: test test_build

.PHONY: test_build_darwin_arm64
test_build_darwin_arm64:
	GOOS=darwin GOARCH=arm64 go test -c -o /dev/null

.PHONY: test_build_darwin_amd64
test_build_darwin_amd64:
	GOOS=darwin GOARCH=amd64 go test -c -o /dev/null

.PHONY: test_build_linux_arm64
test_build_linux_arm64:
	GOOS=linux GOARCH=arm64 go test -c -o /dev/null

.PHONY: test_build_linux_amd64
test_build_linux_amd64:
	GOOS=linux GOARCH=amd64 go test -c -o /dev/null

.PHONY: test_build_windows_amd64
test_build_windows_amd64:
	GOOS=windows GOARCH=amd64 go test -c -o /dev/null

.PHONY: test_build_windows_arm64
test_build_windows_arm64:
	GOOS=windows GOARCH=arm64 go test -c -o /dev/null

.PHONY: test_build_freebsd_amd64
test_build_freebsd_amd64:
	GOOS=freebsd GOARCH=amd64 go test -c -o /dev/null

.PHONY: test_build_openbsd_amd64
test_build_openbsd_amd64:
	GOOS=openbsd GOARCH=amd64 go test -c -o /dev/null

.PHONY: test_build_netbsd_amd64
test_build_netbsd_amd64:
	GOOS=netbsd GOARCH=amd64 go test -c -o /dev/null

.PHONY: test_build_dragonfly_amd64
test_build_dragonfly_amd64:
	GOOS=dragonfly GOARCH=amd64 go test -c -o /dev/null

.PHONY: test_build_solaris_amd64
test_build_solaris_amd64:
	GOOS=solaris GOARCH=amd64 go test -c -o /dev/null

.PHONY: test_build_wasip1_wasm
test_build_wasip1_wasm:
	GOOS=wasip1 GOARCH=wasm go test -c -o /dev/null

.PHONY: test_build_aix_ppc64
test_build_aix_ppc64:
	GOOS=aix GOARCH=ppc64 go test -c -o /dev/null

.PHONY: test_build_js_wasm
test_build_js_wasm:
	GOOS=js GOARCH=wasm go test -c -o /dev/null

# TODO: clean this up and add all supported targets
#
# Test that we can build fastwalk on multiple platforms
.PHONY: test_build
test_build: \
	test_build_aix_ppc64 \
	test_build_darwin_amd64 \
	test_build_darwin_arm64 \
	test_build_dragonfly_amd64 \
	test_build_freebsd_amd64 \
	test_build_js_wasm \
	test_build_linux_amd64 \
	test_build_linux_arm64 \
	test_build_netbsd_amd64 \
	test_build_openbsd_amd64 \
	test_build_solaris_amd64 \
	test_build_wasip1_wasm \
	test_build_windows_amd64 \
	test_build_windows_arm64

.PHONY: test
test: # runs all tests against the package with race detection and coverage percentage
	@go test -race -cover ./...

.PHONY: quick
quick: # runs all tests without coverage or the race detector
	@go test ./...

bin/nilaway:
	@mkdir -p $(MAKEFILE_DIR)/bin
	@GOBIN=$(GOBIN) go install go.uber.org/nilaway/cmd/nilaway@latest

.PHONY: nilaway
nilaway: bin/nilaway
	@$(GOBIN)/nilaway -test=false ./...

.PHONY: nilaway
nilaway_tests: bin/nilaway
	@$(GOBIN)/nilaway -test=true ./...

bin/golangci-lint:
	@mkdir -p $(GOBIN)
	@GOBIN=$(GOBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: golangci-lint
golangci-lint: bin/golangci-lint
	@$(GOBIN)/golangci-lint run ./...

.PHONY: bench
bench:
	go test -run '^$$' -bench . -benchmem ./...

.PHONY: bench_comp
bench_comp:
	@go run ./scripts/bench_comp.go

.PHONY: clean
clean:
	@rm -rf $(MAKEFILE_DIR)/bin
	@go clean

