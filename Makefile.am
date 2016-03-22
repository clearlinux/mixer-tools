dist_bin_SCRIPTS = \
	add-rpms.sh \
	create-update.sh \
	build-chroots.sh \
	init-versions.sh \
	update-bundles.sh

configdir = $(datadir)/defaults/mixer
dist_config_DATA = yum.conf.in
EXTRA_DIST = COPYING

release:
	@git rev-parse v$(PACKAGE_VERSION) &> /dev/null; \
	if [ "$$?" -eq 0 ]; then \
		echo "Error: Release $(PACKAGE_VERSION) already exists."; \
		echo "Bump version in configure.ac before releasing."; \
		exit 1; \
	fi
	@git tag -a -m "Release $(PACKAGE_VERSION)" v$(PACKAGE_VERSION)
	@printf "\nNew release $(PACKAGE_VERSION) tagged!\n\n"
