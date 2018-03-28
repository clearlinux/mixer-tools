#!/bin/bash
rm -rf initramfs
mkdir -p initramfs/{bin,dev,etc,lib64/haswell,usr/lib64/haswell,mnt/root,mnt/data,proc,root,sbin,sys}

mixversion=$(cat mixversion)
echo $mixversion

cp -a update/image/$mixversion/full/usr/lib64/{libpthread.so.0,librt.so.1,libacl.so.1,libattr.so.1,libcap.so.2,ld-linux-x86-64.so.2,libmount.so.1,libblkid.so.1,libuuid.so.1,libcryptsetup.so.4,libpopt.so.0,libdevmapper.so.1.02,libgcrypt.so.20,libudev.so.1,libgpg-error.so.0,ld-2.27.so,libacl.so.1.1.0,libattr.so.1.1.0,libblkid.so.1.1.0,libcap.so.2.25,libcryptsetup.so.4.7.0,libgcrypt.so.20.2.2,libgpg-error.so.0.22.0,libmount.so.1.1.0,libpopt.so.0.0.0,libpthread-2.27.so,librt-2.27.so,libudev.so.1.6.6,libuuid.so.1.3.0,libtinfo.so.6,libtinfo.so.6.1} initramfs/usr/lib64/

cp -a update/image/$mixversion/full/usr/lib64/{haswell/libc.so.6,haswell/libm.so.6,haswell/libgcc_s.so.1,haswell/libc-2.27.so,haswell/libm-2.27.so} initramfs/usr/lib64/haswell/


cp -a update/image/$mixversion/full/lib64/{libpthread.so.0,librt.so.1,libacl.so.1,libattr.so.1,libcap.so.2,ld-linux-x86-64.so.2,libmount.so.1,libblkid.so.1,libuuid.so.1,libcryptsetup.so.4,libpopt.so.0,libdevmapper.so.1.02,libgcrypt.so.20,libudev.so.1,libgpg-error.so.0,ld-2.27.so,libacl.so.1.1.0,libattr.so.1.1.0,libblkid.so.1.1.0,libcap.so.2.25,libcryptsetup.so.4.7.0,libgcrypt.so.20.2.2,libgpg-error.so.0.22.0,libmount.so.1.1.0,libpopt.so.0.0.0,libpthread-2.27.so,librt-2.27.so,libudev.so.1.6.6,libuuid.so.1.3.0,libtinfo.so.6,libtinfo.so.6.1} initramfs/lib64/

cp -a update/image/$mixversion/full/lib64/{haswell/libc.so.6,haswell/libm.so.6,haswell/libgcc_s.so.1,haswell/libc-2.27.so,haswell/libm-2.27.so} initramfs/lib64/haswell/


cp -a update/image/$mixversion/full/sbin/{bash,sh,coreutils,mount,umount,veritysetup,switch_root,cat,sleep} initramfs/sbin/
cp -a update/image/$mixversion/full/bin/{bash,sh,coreutils,mount,umount,veritysetup,switch_root,cat,sleep} initramfs/bin/

#cp init initramfs/
