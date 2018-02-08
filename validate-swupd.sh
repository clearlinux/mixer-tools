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
		popd
		exit 1
	fi
	mixer bundle add --all-upstream
	if [[ $? -ne 0 ]]; then
		echo "failed to add mix bundles"
		popd
		exit 1
	fi
	mixer build chroots --new-swupd --new-chroots --chroot-workers 20
	if [[ $? -ne 0 ]]; then
		echo "failed to build mix chroots"
		popd
		exit 1
	fi
	mixer build update --new-swupd --fullfile-workers 56 --format $3
	if [[ $? -ne 0 ]]; then
		echo "failed to build mix update"
		popd
		exit 1
	fi
	mixer build delta-packs --previous-versions 1 --new-swupd --delta-workers 6
	if [[ $? -ne 0 ]]; then
		echo "failed to build delta packs"
		popd
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
		popd
		exit 1
	fi
	swupd bundle-add bootloader os-core-update kernel-native -F $3 --path os-$i-install -u file://$(pwd)/update/www -C $(pwd)/Swupd_Root.pem
	if [[ $? -ne 0 ]]; then
		echo "failed bundle-add"
		popd
		exit 1
	fi
	swupd bundle-remove -F $3 kernel-native --path os-$i-install  -u file://$(pwd)/update/www -C $(pwd)/Swupd_Root.pem
	if [[ $? -ne 0 ]]; then
		echo "failed bundle-remove" 
		popd
		exit 1
	fi
	swupd verify -F $3 --path os-$i-install -u file://$(pwd)/update/www -C $(pwd)/Swupd_Root.pem
	if [[ $? -ne 0 ]]; then
		popd
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
	swupd update --path os-$i-install -u file://$(pwd)/update/www -C $(pwd)/Swupd_Root.pem
	if [[ $? -ne 0 ]]; then
		popd
		exit 1
	fi
	set +x

	rm -rf os-$i-install
done

popd
