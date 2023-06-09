.PHONY: proto test lint wasirun

GOPATH ?= $(shell $(GO) env GOPATH)
wasirun = $(GOPATH)/bin/wasirun

packages.dir = $(wildcard */)
packages.test = $(packages.dir:/=.test)
test: proto wasirun $(packages.test)
	@for pkg in $(packages.test); do \
		tmp=$$(mktemp); \
		$(wasirun) $$pkg > $$tmp; \
		if (($$?)); then cat $$tmp; exit 1; else printf "ok\tgithub.com/stealthrocket/net/$$pkg\n"; fi \
	done

# go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
grpc.proto = $(wildcard grpc/*.proto)
grpc.pb.go = $(grpc.proto:.proto=.pb.go)
proto: $(grpc.pb.go)

wasirun: $(wasirun)

$(wasirun):
	$(GO) install github.com/stealthrocket/wasi-go/cmd/wasirun@latest

%.test: %/
	cd $< && GOARCH=wasm GOOS=wasip1 $(GO) test -c -o ../$(notdir $@)

%.pb.go: %.proto
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative $<
