#!/bin/bash
boot_dev=$1
initramfs_fname=$2
echo $boot_dev
echo $initramfs_fname
mount $boot_dev mnt

#rm -rf initramfs
#mkdir initramfs
#cp init initramfs/
cd initramfs
chmod +x init
find . | cpio -o -H newc | gzip > ../mnt/EFI/$initramfs_fname
#find . -print0 | cpio --null -ov --format=newc | gzip -9 > ../mnt/EFI/$initramfs_fname
ls ../mnt/EFI
cat ../mnt/loader/entries/Clear*
cd ..
#echo "initrd EFI/$initramfs_fname"  >> ../mnt/loader/entries/Clear-linux-native-4.15.4-534.conf
umount mnt
