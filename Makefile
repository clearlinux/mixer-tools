include Makefile.bats

.NOTPARALLEL:

VERSION=6.1.5
GO_PACKAGE_PREFIX := github.com/clearlinux/mixer-tools
GOPATH ?= ${HOME}/go
gopath = $(shell go env GOPATH)

.PHONY: build install clean check

.DEFAULT_GOAL := build

build:
	go install -ldflags="-X ${GO_PACKAGE_PREFIX}/builder.Version=${VERSION}" ${GO_PACKAGE_PREFIX}/mixer
	go install ${GO_PACKAGE_PREFIX}/mixin
	go install ${GO_PACKAGE_PREFIX}/swupd-extract
	go install ${GO_PACKAGE_PREFIX}/swupd-inspector
	go install ${GO_PACKAGE_PREFIX}/mixer-completion

install: build
	test -d $(DESTDIR)/usr/bin || install -D -d -m 00755 $(DESTDIR)/usr/bin;
	install -m 00755 $(GOPATH)/bin/mixer $(DESTDIR)/usr/bin/.
	install -m 00755 $(GOPATH)/bin/mixin $(DESTDIR)/usr/bin/.
	install -m 00755 $(GOPATH)/bin/swupd-extract $(DESTDIR)/usr/bin/.
	install -m 00755 $(GOPATH)/bin/swupd-inspector $(DESTDIR)/usr/bin/.
	$(GOPATH)/bin/mixer-completion bash --path $(DESTDIR)/usr/share/bash-completion/completions/mixer
	$(GOPATH)/bin/mixer-completion zsh --path $(DESTDIR)/usr/share/zsh/site-functions/_mixer
	test -d $(DESTDIR)/usr/share/man/man1 || install -D -d -m 00755 $(DESTDIR)/usr/share/man/man1
	install -m 00644 $(MANPAGES) $(DESTDIR)/usr/share/man/man1/

check:
	go test -cover ${GO_PACKAGE_PREFIX}/...

.PHONY: checkcoverage
checkcoverage:
	go test -cover ${GO_PACKAGE_PREFIX}/... -coverprofile=coverage.out
	go tool cover -html=coverage.out

.PHONY: lint
lint:
	@if ! $(gopath)/bin/golangci-lint --version &>/dev/null; then \
		echo "Installing linters..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(gopath)/bin v1.22.2; \
	fi
	@$(gopath)/bin/golangci-lint run --deadline=10m --tests --disable-all \
	--enable=misspell \
	--enable=vet \
	--enable=ineffassign \
	--enable=gofmt \
	$${CYCLO_MAX:+--enable=gocyclo --cyclo-over=$${CYCLO_MAX}} \
	--enable=golint \
	--enable=deadcode \
	--enable=varcheck \
	--enable=structcheck \
	--enable=unused \
	--enable=vetshadow \
	--enable=errcheck \
	./...

clean:
	go clean -i -x ${GO_PACKAGE_PREFIX}/...
	rm -f mixer-tools-*.tar.gz

release:
	@if [ ! -d .git ]; then \
		echo "Release needs to be used from a git repository"; \
		exit 1; \
	fi
	git archive --format=tar.gz --verbose -o mixer-tools-${VERSION}.tar.gz HEAD --prefix=mixer-tools-${VERSION}/

MANPAGES = \
	docs/mixer.1 \
	docs/mixer.add-rpms.1 \
	docs/mixer.build.1 \
	docs/mixer.bundle.1 \
	docs/mixer.config.1 \
	docs/mixer.init.1 \
	docs/mixer.repo.1 \
	docs/mixer.versions.1 \
	docs/mixin.1

man: $(MANPAGES)

% : %.rst
	mkdir -p "$$(dirname $@)"
	rst2man.py "$<" > "$@.tmp" && mv -f "$@.tmp" "$@"
