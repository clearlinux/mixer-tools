# Makefile used to create packages for mixer-tools. It doesn't assume
# that the code is inside a GOPATH, and always copy the files into a
# new workspace to get the work done. Go tools doesn't reliably work
# with symbolic links.
#
# For historical purposes, it also works in a development environment
# when the repository is already inside a GOPATH.
include Makefile.bats

.NOTPARALLEL:

VERSION=5.9.1
GO_PACKAGE_PREFIX := github.com/clearlinux/mixer-tools

.PHONY: gopath

# Strictly speaking we should check if it the directory is inside an
# actual GOPATH, but the directory structure matching is likely enough.
ifeq (,$(findstring ${GO_PACKAGE_PREFIX},${CURDIR}))
LOCAL_GOPATH := ${CURDIR}/.gopath
export GOPATH := ${LOCAL_GOPATH}
gopath:
	@rm -rf ${LOCAL_GOPATH}/src
	@mkdir -p ${LOCAL_GOPATH}/src/${GO_PACKAGE_PREFIX}
	@cp -af * ${LOCAL_GOPATH}/src/${GO_PACKAGE_PREFIX}
	@echo "Prepared a local GOPATH=${GOPATH}"
else
LOCAL_GOPATH :=
GOPATH ?= ${HOME}/go
gopath:
	@echo "Code already in existing GOPATH=${GOPATH}"
endif

.PHONY: build install clean check

.DEFAULT_GOAL := build


build: gopath
	go install -ldflags="-X ${GO_PACKAGE_PREFIX}/builder.Version=${VERSION}" ${GO_PACKAGE_PREFIX}/mixer
	go install ${GO_PACKAGE_PREFIX}/mixin
	go install ${GO_PACKAGE_PREFIX}/swupd-extract
	go install ${GO_PACKAGE_PREFIX}/swupd-inspector
	go install ${GO_PACKAGE_PREFIX}/mixer-completion

install: gopath
	test -d $(DESTDIR)/usr/bin || install -D -d -m 00755 $(DESTDIR)/usr/bin;
	install -m 00755 $(GOPATH)/bin/mixer $(DESTDIR)/usr/bin/.
	install -m 00755 $(GOPATH)/bin/mixin $(DESTDIR)/usr/bin/.
	install -m 00755 $(GOPATH)/bin/swupd-extract $(DESTDIR)/usr/bin/.
	install -m 00755 $(GOPATH)/bin/swupd-inspector $(DESTDIR)/usr/bin/.
	$(GOPATH)/bin/mixer-completion bash --path $(DESTDIR)/usr/share/bash-completion/completions/mixer
	$(GOPATH)/bin/mixer-completion zsh --path $(DESTDIR)/usr/share/zsh/site-functions/_mixer
	test -d $(DESTDIR)/usr/share/man/man1 || install -D -d -m 00755 $(DESTDIR)/usr/share/man/man1
	install -m 00644 $(MANPAGES) $(DESTDIR)/usr/share/man/man1/

check: gopath
	go test -cover ${GO_PACKAGE_PREFIX}/...

.PHONY: checkcoverage
checkcoverage:
	go test -cover ${GO_PACKAGE_PREFIX}/... -coverprofile=coverage.out
	go tool cover -html=coverage.out

.PHONY: lint
lint: gopath
	@golangci-lint run --deadline=10m --tests --disable-all \
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
ifeq (,${LOCAL_GOPATH})
	go clean -i -x
else
	rm -rf ${LOCAL_GOPATH}
endif
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
