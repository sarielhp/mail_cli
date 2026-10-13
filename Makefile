# Every target delegates to the canonical commands in tools/; the project
# contract they read is tools/settings.rb. Use tools/commit directly for
# messages with shell metacharacters.
.PHONY: all build gate check ci lint snapshot commit bump install release test test-live interactive audit review static-analysis push clean

all: build

build:
	tools/build

gate:
	tools/gate

check:
	tools/check

ci:
	tools/gate --full

lint:
	tools/lint

snapshot:
	tools/snapshot $(if $(msg),"$(msg)")

commit:
	tools/commit "$(msg)"

bump:
	tools/bump

install:
	tools/install

# make release msg="..." [NO_INSTALL=1]; msg may be omitted on a clean tree.
release:
	tools/release $(if $(NO_INSTALL),--no-install) $(if $(msg),"$(msg)")

test:
	go test -timeout 180s ./...

test-live:
	ruby tools/test_live.rb

# Drives the TUI in kitty on a private Xvfb display; too slow for the gate.
# Fails, naming the packages, when a required tool is missing.
interactive:
	MAIL_CLI_INTERACTIVE=1 go test ./interactive/ -v -count=1

audit:
	tools/go-audit --strict --quiet .

review: static-analysis
static-analysis:
	tools/go-static-analysis

push:
	tools/push

clean:
	go clean
	rm -rf bin mail_cli
