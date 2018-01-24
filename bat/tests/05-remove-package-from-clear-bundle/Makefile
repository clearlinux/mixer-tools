.PHONY: check clean

check:
	bats ./run.bats

CLEANDIRS = ./update ./test-chroot ./logs ./.repos ./bundles ./update ./mix-bundles ./clr-bundles ./local ./results ./repodata ./rpms ./upstream-bundles ./local-bundles
CLEANFILES = ./*.log ./run.bats.trs ./yum.conf.in ./builder.conf ./.{c,m}* *.pem .yum-mix.conf mixversion upstreamurl upstreamversion mixbundles
clean:
	sudo rm -rf $(CLEANDIRS) $(CLEANFILES)
