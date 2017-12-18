PROJECT_ROOT := src/
VERSION = 3.2.0

.DEFAULT_GOAL := all

CHANGES := $(shell go fmt ./... 2>&1)

# Ensure code is compliant
compliant:
	if [ "$(CHANGES)" ]; then echo -e "Error, go fmt ./... updated:\n$(CHANGES)"; exit 1 ; fi
	go vet ./...

install:
	test -d $(DESTDIR)/usr/bin || install -D -d -m 00755 $(DESTDIR)/usr/bin;
	install -m 00755 $(GOPATH)/bin/mixer $(DESTDIR)/usr/bin/.
	install -m 00755 pack-maker.sh $(DESTDIR)/usr/bin/mixer-pack-maker.sh
	install -m 00755 superpack-maker.sh $(DESTDIR)/usr/bin/mixer-superpack-maker.sh
	install -D -m 00644 yum.conf.in $(DESTDIR)/usr/share/defaults/mixer/yum.conf.in

release:
	git archive --format=tar.gz --verbose -o mixer-tools-$(VERSION).tar.gz HEAD --prefix=mixer-tools-$(VERSION)/

all: compliant
	go install ./...

clean:
	rm -rf $(GOPATH)/bin/mixer
	rm -rf mixer-tools-*.tar.gz
