usage="$(basename "$0") FROM TO FORMAT

where:
	FROM    upstream version to create builds from
	TO      upstream version to create builds to
	FORMAT  format number for upstream builds (must match upstream format)"

if [ $# -ne 3 ]; then
	echo "$usage"
	exit
fi

is_published() {
	format=$(curl -f https://download.clearlinux.org/update/$1/format)
}

bundle_tagged() {
	curl -LIf https://github.com/clearlinux/clr-bundles/archive/$1.tar.gz &> /dev/null
	return $?
}

# create mix content
mkdir validate-swupd
pushd validate-swupd
for i in $(seq $1 10 $2); do
	echo "checking ${i}..."
	if ! is_published $i; then
		echo "${i} is not published"
		continue
	fi

	echo "${i} was published upstream!"
	if ! bundle_tagged $i; then
		echo "${i} not tagged upstream"
		continue
	fi
	echo "========================================================"
	echo "Building based on ${i}"
	echo "========================================================"

	mixer init --clear-version $i --mix-version "${i}"
	if [[ $? -ne 0 ]]; then
		echo "failed to init mix"
		exit 1
	fi
	mixer bundle add --all-upstream
	if [[ $? -ne 0 ]]; then
		echo "failed to add mix bundles"
		exit 1
	fi
	echo "helloworld" >> local-packages
	mixer bundle add helloworld
	if [[ $? -ne 0 ]]; then
		echo "failed to add mix bundles"
		exit 1
	fi
	# increase the number of bundle-workers on larger systems
	# keep in mind that this is network bound due to dnf installs
	# of upstream tarballs
	mixer build bundles --bundle-workers 8
	if [[ $? -ne 0 ]]; then
		echo "failed to build mix bundles"
		exit 1
	fi
	# increase the number of fullfile-workers on larger systems
	mixer build update --fullfile-workers 8 --min-version $1 --format $3
	if [[ $? -ne 0 ]]; then
		echo "failed to build mix update"
		exit 1
	fi
	# increase the number of delta-workers on larger systems
	# this is memory-bound instead of cpu-bound
	mixer build delta-packs --previous-versions 1 --delta-workers 4
	if [[ $? -ne 0 ]]; then
		echo "failed to build delta packs"
		exit 1
	fi

	echo ""
	echo "Finished build ${i}"
	echo "////////////////////////////////////////////////////////"
done

# validate install
for i in $(ls update/www); do
	if [[ $i -eq 0 ]]; then
		continue
	fi

	if [[ $i -eq "version" ]]; then
		continue
	fi
	echo "starting validation loop"

	mkdir -p os-$i-install
	# clean dir for each round
	rm -rf /var/lib/swupd
	swupd verify --install --path os-$i-install -m $i -F $3 -u file://$(pwd)/update/www -C $(pwd)/Swupd_Root.pem
	if [[ $? -ne 0 ]]; then
		echo "failed verify --install"
		exit 1
	fi
	swupd bundle-add bootloader os-core-update kernel-native helloworld -F $3 --path os-$i-install -u file://$(pwd)/update/www -C $(pwd)/Swupd_Root.pem
	if [[ $? -ne 0 ]]; then
		echo "failed bundle-add"
		exit 1
	fi

	pushd os-$i-install
	$(pwd)/usr/bin/helloworld
	if [[ $? -ne 0 ]]; then
		echo "failed helloworld"
		exit 1
	fi
	popd

	swupd bundle-remove -F $3 kernel-native --path os-$i-install  -u file://$(pwd)/update/www -C $(pwd)/Swupd_Root.pem
	if [[ $? -ne 0 ]]; then
		echo "failed bundle-remove" 
		exit 1
	fi
	swupd verify -F $3 --path os-$i-install -u file://$(pwd)/update/www -C $(pwd)/Swupd_Root.pem
	if [[ $? -ne 0 ]]; then
		exit 1
	fi
	# update latest file
	for j in $(ls update/www); do
		if [[ $j -eq "version" ]]; then
			continue
		fi

		if [[ $j -gt $i ]]; then
			next=$j
			break
		fi
	done

	set -x
	echo $next | sudo tee $(pwd)/update/www/version/format$3/latest
	swupd update --path "os-$i-install" -F "$3" -u "file://$PWD/update/www" -C "$PWD/Swupd_Root.pem"
	if [[ $? -ne 0 ]]; then
		exit 1
	fi
	set +x

	rm -rf os-$i-install
done
