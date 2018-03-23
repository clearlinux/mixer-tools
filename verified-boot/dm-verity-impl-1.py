#!/usr/bin/env python3
import subprocess
import time
import sys
import os
import urllib.request as request
import json
import fnmatch
import re

disk_image = os.path.abspath(sys.argv[1])
cmd = "losetup -f -P --show {0}".format(disk_image)
print("Executing: " + cmd)
try:
    dev = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
except Exception:
    raise Exception("{0}: {1}".format(cmd, sys.exc_info()))
print(dev[len(dev) - 1])
boot_num = 1
rootfs_num = 3
data_num = 4
hash_num = 5

boot_dev = dev[0] + "p" + str(boot_num)
rootfs_dev = dev[0] + "p" + str(rootfs_num)
data_dev = dev[0] + "p" + str(data_num)
hash_dev = dev[0] + "p" + str(hash_num)
verity_name = "root"

print("Creating data files in " + data_dev)
subprocess.check_output("rm -rf mnt".split(" "))
subprocess.check_output("mkdir mnt".split(" "))
subprocess.check_output("mount {0} mnt".format(data_dev).split(" "))

subprocess.check_output("touch mnt/file1.sh".split(" "))
try:
    outfile = open('mnt/file1.sh','w')
    outfile.write("echo Testing dm-verity data1...")
    outfile.close()
except IOError:
    print("I/O error")

subprocess.check_output("touch mnt/file2.sh".split(" "))
try:
    outfile = open('mnt/file2.sh','w')
    outfile.write("echo Testing dm-verity data2...")
    outfile.close()
except IOError:
    print("I/O error")

subprocess.check_output("umount mnt".split(" "))

cmd = "veritysetup --verbose --data-block-size=1024 --hash-block-size=1024 format {0} {1}".format(data_dev, hash_dev)
print("Executing: " + cmd)
try:
    res = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
except Exception:
    raise Exception("{0}: {1}".format(cmd, sys.exc_info()))

print(res[len(res) - 1])

salt_str = res[len(res) - 3]
root_hash_str = res[len(res) - 2]
print(salt_str)
print(root_hash_str)
salt = salt_str.replace('Salt:            	','')
root_hash = root_hash_str.replace('Root hash:      	','')
print(salt)
print(root_hash)


kernel_cmdline = "quiet systemd.verity=yes roothash=" + root_hash + " systemd.verity_root_data=/dev/sda" + str(data_num) + " systemd.verity_root_hash=/dev/sda" + str(hash_num)
subprocess.check_output("mount {0} mnt".format(boot_dev).split(" "))

for fname in os.listdir('mnt/loader/entries/'):
    if (fnmatch.fnmatch(fname, 'Clear-*')):
        path = "mnt/loader/entries/" + fname
        try:
            outfile = open(path, 'r')
            content = outfile.read()
            print(content)
            outfile.close()
        except IOError:
            print("I/O error")

        content = re.sub(r"quiet", kernel_cmdline, content)
        print(content)

        try:
            outfile = open(path, 'w')
            outfile.write(content)
            outfile.close()
        except IOError:
            print("I/O error")

subprocess.check_output("umount mnt".split(" "))
subprocess.check_output("rm -rf mnt".split(" "))

cmd = "veritysetup --verbose verify {0} {1} {2}".format(data_dev, hash_dev, root_hash)

print("Executing: " + cmd)
try:
   res = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
except Exception:
   raise Exception("{0}: {1}".format(cmd, sys.exc_info()))
print(res[len(res) - 1])

#cmd = "veritysetup --verbose --data-block-size=1024 --hash-block-size=1024 create " + verity_name + " {0} {1} {2}".format(data_dev, hash_dev, root_hash)
#print("Executing: " + cmd)
#try:
#   res = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
#except Exception:
#   raise Exception("{0}: {1}".format(cmd, sys.exc_info()))
#print(res[len(res) - 1])

print("Done!")
