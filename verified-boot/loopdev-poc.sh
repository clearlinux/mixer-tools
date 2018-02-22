#!/bin/bash

dd if=/dev/zero of=drive.img bs=1024 count=10240

#losetup /dev/loop0 /home/reaganlo/mix/release.img
losetup /dev/loop0 drive.img

cat << EOF | fdisk /dev/loop0
o
n
p
1

+2M
n
p
2


p
w
q
EOF

kpartx -a /dev/loop0

mkfs.ext4 -b 4096 /dev/mapper/loop0p2

mkdir veritymnt
mount /dev/mapper/loop0p2 veritymnt
echo a > veritymnt/f1.txt
echo b > veritymnt/f2.txt
umount veritymnt

echo "Performing veritysetup format..."
veritysetup format /dev/mapper/loop0p2 /dev/mapper/loop0p1 | tee out
ROOT_HASH=$(cat out | grep "^Root hash:" | sed -e 's/.*\s\(\S*\)$/\1/')
echo $ROOT_HASH
rm out

echo "Performing veritysetup create..."
veritysetup create loopverity /dev/mapper/loop0p2 /dev/mapper/loop0p1 ${ROOT_HASH}
mount /dev/mapper/loopverity veritymnt
