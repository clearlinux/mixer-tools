PROJECT_ROOT := src/
VERSION = 3.0

.DEFAULT_GOAL := all

# Locate testables:
_TESTABLES = $(shell find src/ -name '*_test.go'|xargs -I{} dirname {}|sed 's/src\///g'|uniq|sort)
_COMPLIABLE = $(shell find src/ -name '*.go' | xargs -I{} dirname {}|sed 's/src\///g'|uniq|sort)

GO_TESTS = \
	$(addsuffix .test,$(_TESTABLES))

BUILDABLES = \
	mixer.build

include Makefile.gobuild

# We want to add compliance for all built binaries
_CHECK_COMPLIANCE = $(addsuffix .compliant,$(_COMPLIABLE))

# Ensure our own code is compliant..
compliant: $(_CHECK_COMPLIANCE)
install: $(BINS)
	test -d $(DESTDIR)/usr/bin || install -D -d -m 00755 $(DESTDIR)/usr/bin; \
	install -m 00755 bin/* $(DESTDIR)/usr/bin/.
	install -m 00755 pack-maker.sh $(DESTDIR)/usr/bin/mixer-pack-maker.sh
	install -m 00755 superpack-maker.sh $(DESTDIR)/usr/bin/mixer-superpack-maker.sh
	install -D -m 00644 yum.conf.in $(DESTDIR)/usr/share/defaults/mixer/yum.conf.in

release:
	git archive --format=tar.gz --verbose -o mixer-$(VERSION).tar.gz HEAD --prefix=mixer-$(VERSION)/

all: compliant $(BUILDABLES)
