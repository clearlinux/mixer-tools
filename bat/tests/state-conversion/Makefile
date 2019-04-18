.PHONY: check clean

check:
	bats ./run.bats

CLEANDIRS = ./update ./test-chroot ./logs ./.repos ./bundles ./update ./mix-bundles ./clr-bundles ./local-yum ./results ./repodata ./local-rpms ./upstream-bundles ./local-bundles
CLEANFILES = ./*.log ./run.bats.trs ./yum.conf.in ./builder.conf ./mixer.state.bkp ./mixer.state ./.{c,m}* *.pem .yum-mix.conf mixversion upstreamurl upstreamversion mixbundles current.state
clean:
	sudo rm -rf $(CLEANDIRS) $(CLEANFILES)
