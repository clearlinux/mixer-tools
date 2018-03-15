#!/bin/bash
boot_dev=$1
initramfs_fname=$2
echo $boot_dev
echo $initramfs_fname
mount $boot_dev mnt
cd initramfs
find . -print0 | cpio --null -ov --format=newc | gzip -9 > ../mnt/EFI/$initramfs_fname
echo "initrd EFI/$initramfs_fname"  >> ../mnt/loader/entries/Clear-linux-native-4.15.4-534.conf
cd ..
#umount mnt
