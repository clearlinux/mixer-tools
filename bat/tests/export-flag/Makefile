.PHONY: check clean

check:
	bats ./run.bats

CLEANDIRS = ./update ./test-chroot ./logs ./.repos ./bundles ./update ./mix-bundles ./clr-bundles ./local-yum ./results ./repodata ./local-rpms ./upstream-bundles ./local-bundles ./custom-content
CLEANFILES = ./*.log ./run.bats.trs ./yum.conf.in ./builder.conf ./mixer.state ./.{c,m}* *.pem .yum-mix.conf mixversion upstreamurl upstreamversion mixbundles
clean:
	sudo rm -rf $(CLEANDIRS) $(CLEANFILES)
