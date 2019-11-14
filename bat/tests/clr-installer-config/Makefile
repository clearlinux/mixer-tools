.PHONY: check clean

check:
	bats ./run.bats

CLEANDIRS = ./update ./test-chroot ./logs ./.repos ./bundles ./update ./mix-bundles ./clr-bundles ./local-yum ./results ./repodata ./local-rpms ./upstream-bundles ./local-bundles ./clr-installer*
CLEANFILES = ./*.log ./run.bats.trs ./yum.conf.in ./builder.conf ./mixer.state ./.{c,m}* *.pem .yum-mix.conf mixversion upstreamurl upstreamversion mixbundles release-image-config.yaml release.img release.iso pre-install-clr-installer.yaml release-image-config.log
clean:
	sudo rm -rf $(CLEANDIRS) $(CLEANFILES)
