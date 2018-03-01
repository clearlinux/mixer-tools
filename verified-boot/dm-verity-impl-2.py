#!/usr/bin/env python3
import subprocess
import time
import sys
import os
import urllib.request as request
import json

#disk_image = "release.img"
#infile_path = "file://" + os.path.abspath(sys.argv[1])
#json_file = request.urlopen(infile_path)
#template = json.loads(json_file.read().decode("utf-8"))
#disk_image = os.path.abspath(template["PartitionLayout"][0]["disk"])

disk_image = os.path.abspath(sys.argv[1])

cmd = "kpartx -a -v " + disk_image
try:
    res = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
except Exception:
    raise Exception("{0}: {1}".format(cmd, sys.exc_info()))
data_num = 4
hash_num = 5
store_num = 6
verity_name = "vloop1"

data_mapper = "/dev/mapper/" + res[data_num - 1].split()[2]
hash_mapper = "/dev/mapper/" + res[hash_num - 1].split()[2]
store_mapper = "/dev/mapper/" + res[store_num - 1].split()[2]


#command1 = "losetup -f -P --show {0}".format(disk_image)
#print("Executing: " + command1)
#try:
#	dev = subprocess.check_output(command1.split(" ")).decode("utf-8")\
#		                                             .splitlines()
#except Exception:
#	raise Exception("losetup command failed: {0}: {1}"
#                            .format(command1, sys.exc_info()))
#if len(dev) != 1:
#	raise Exception("losetup failed to create loop device")
#time.sleep(1)

#data_dev = dev[0] + "p4"
#hash_dev = dev[0] + "p5"
#store_dev = dev[0] + "p6"

#data_mapper = data_dev.replace("dev", "dev/mapper")
#hash_mapper = hash_dev.replace("dev", "dev/mapper")
#store_mapper = store_dev.replace("dev", "dev/mapper")

#subprocess.check_output("kpartx -l {0}".format(disk_image).split(" "))
#subprocess.check_output("mkfs.ext4 -b 4096 {0}".format(data_mapper).split(" "))

print("Creating data files in " + data_mapper)
subprocess.check_output("rm -rf mnt".split(" "))
subprocess.check_output("mkdir mnt".split(" "))
subprocess.check_output("mount {0} mnt".format(data_mapper).split(" "))

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

cmd = "veritysetup --verbose --data-block-size=1024 --hash-block-size=1024 format {0} {1}".format(data_mapper, hash_mapper)
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
root_hash = root_hash_str.replace('Root hash:      	','')
salt = salt_str.replace('Salt:            	','')

subprocess.check_output("mount {0} mnt".format(store_mapper).split(" "))
print("Writing the root_hash to hash.txt in " + store_mapper)
subprocess.check_output("touch mnt/hash.txt".split(" "))
try:
    outfile = open('mnt/hash.txt','w')
    outfile.write(root_hash)
    outfile.close()
except IOError:
    print("I/O error")

print("Writing the salt to salt.txt in " + store_mapper)
subprocess.check_output("touch mnt/salt.txt".split(" "))
try:
    outfile = open('mnt/salt.txt','w')
    outfile.write(salt)
    outfile.close()
except IOError:
    print("I/O error")

print("Writing veritysetup create command to vcreate.sh in " + store_mapper)
subprocess.check_output("touch mnt/vcreate.sh".split(" "))
try:
    outfile = open('mnt/vcreate.sh','w')
    outfile.write("veritysetup --verbose --data-block-size=1024 --hash-block-size=1024 create " + verity_name + " /dev/sda" + str(data_num) + " /dev/sda" + str(hash_num) + " " + root_hash + "\n")
    outfile.write("mkdir /mnt/vloop\n")
    outfile.write("mount /dev/mapper/" + verity_name + " /mnt/vloop")
    outfile.close()
except IOError:
    print("I/O error")
    
subprocess.check_output("umount mnt".split(" "))

cmd = "veritysetup --verbose verify {0} {1} {2}".format(data_mapper, hash_mapper, root_hash)
print("Executing: " + cmd)
try:
    res = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
except Exception:
    raise Exception("{0}: {1}".format(cmd, sys.exc_info()))
print(res[len(res) - 1])


#cmd = "veritysetup --verbose --data-block-size=1024 --hash-block-size=1024 create {0} {1} {2} {3}".format(verity_name, data_mapper, hash_mapper, root_hash)
#print("Executing: " + cmd)
#try:
#    res = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
#except Exception:
#    raise Exception("{0}: {1}".format(cmd, sys.exc_info()))
#print(res[len(res) - 1])
#
#cmd = "mount /dev/mapper/" + verity_name + " mnt"
#print("Executing: " + cmd)
#subprocess.check_output(cmd.split(" "))
print("Done!")

#cmd = "blockdev --getsz {0}".format(data_mapper)
#print("Executing: " + cmd)
#res = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
#bsize = res[0]
#print("data_mapper block size = " + bsize)
#arg = "0 " + str(bsize) + " verity 1 " + str(data_mapper) + " " + str(hash_mapper) + " 4096 4096 522240 1 sha256 " + str(root_hash) + " " + str(salt)
#cmd = "dmsetup create {0} -r --table \"{1}\"".format(verity_name, arg)
#print("Executing: " + cmd)
#try:
#   res = subprocess.check_output(cmd.split(" ")).decode("utf-8").splitlines()
#except Exception:
#   raise Exception("{0}: {1}".format(cmd, sys.exc_info()))
