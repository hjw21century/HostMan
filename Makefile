.PHONY: build build-agent run clean deps all

BINARY_SERVER=hostman-server
BINARY_AGENT=hostman-agent
GOPATH_BIN=/usr/local/go/bin
GO=PATH=$(GOPATH_BIN):$$PATH go

all: build build-agent

deps:
	cd /root/.openclaw/workspace/hostman && $(GO) mod tidy

build: deps
	cd /root/.openclaw/workspace/hostman && CGO_ENABLED=1 $(GO) build -o bin/$(BINARY_SERVER) ./cmd/server
	@echo "✅ Built bin/$(BINARY_SERVER)"

build-agent: deps
	cd /root/.openclaw/workspace/hostman && $(GO) build -o bin/$(BINARY_AGENT) ./cmd/agent
	@echo "✅ Built bin/$(BINARY_AGENT)"

run: build
	cd /root/.openclaw/workspace/hostman && ./bin/$(BINARY_SERVER) -debug -templates web/templates

install-server: build
	mkdir -p /opt/hostman/data /opt/hostman/web
	cp bin/$(BINARY_SERVER) /opt/hostman/
	cp -r web/templates /opt/hostman/web/
	cp deploy/hostman-server.service /etc/systemd/system/
	systemctl daemon-reload
	@echo "✅ Server installed. Run: systemctl enable --now hostman-server"

install-agent: build-agent
	mkdir -p /opt/hostman
	cp bin/$(BINARY_AGENT) /opt/hostman/
	@echo "✅ Agent binary installed to /opt/hostman/"
	@echo "   Run deploy/install-agent.sh for full setup"

clean:
	rm -rf bin/ hostman.db
