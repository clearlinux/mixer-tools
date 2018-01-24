# Makefile used to create packages for mixer-tools. It doesn't assume
# that the code is inside a GOPATH, and always copy the files into a
# new workspace to get the work done. Go tools doesn't reliably work
# with symbolic links.
#
# For historical purposes, it also works in a development environment
# when the repository is already inside a GOPATH.
include Makefile.bats

.NOTPARALLEL:

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
	go install ${GO_PACKAGE_PREFIX}/mixer

install: gopath
	test -d $(DESTDIR)/usr/bin || install -D -d -m 00755 $(DESTDIR)/usr/bin;
	install -m 00755 $(GOPATH)/bin/mixer $(DESTDIR)/usr/bin/.
	install -m 00755 pack-maker.sh $(DESTDIR)/usr/bin/mixer-pack-maker.sh
	install -m 00755 superpack-maker.sh $(DESTDIR)/usr/bin/mixer-superpack-maker.sh
	install -D -m 00644 yum.conf.in $(DESTDIR)/usr/share/defaults/mixer/yum.conf.in

check: gopath
	go test -cover ${GO_PACKAGE_PREFIX}/...

# TODO: when Go 1.10 comes out we will have support for passing multiple packages
# to coverprofile, so there will be no need to pass an individual package.
# At that time we can merge this target into check and run it against all
# packages every time.
.PHONY: checkcoverage
checkcoverage: gopath
ifeq (,${PKG})
	$(error PKG is not set, try make PKG=swupd checkcoverage)
else
	go test -cover ${GO_PACKAGE_PREFIX}/${PKG} -coverprofile=coverage.out
	go tool cover -html=coverage.out
endif

.PHONY: lint
lint: gopath
	@gometalinter.v2 --deadline=10m --tests --vendor --disable-all \
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
	@VERSION=$$(grep -e 'const Version' builder/builder.go | cut -d '"' -f 2) ; \
	if [ -z "$$VERSION" ]; then \
		echo "Couldn't extract version number from the source code"; \
		exit 1; \
	fi; \
	git archive --format=tar.gz --verbose -o mixer-tools-$$VERSION.tar.gz HEAD --prefix=mixer-tools-$$VERSION/

