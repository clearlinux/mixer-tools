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
command1 = "losetup -f -P --show {0}".format(disk_image)
print("Starting " + command1)
try:
	dev = subprocess.check_output(command1.split(" ")).decode("utf-8")\
		                                             .splitlines()
except Exception:
	raise Exception("losetup command failed: {0}: {1}"
                            .format(command1, sys.exc_info()))
if len(dev) != 1:
	raise Exception("losetup failed to create loop device")
time.sleep(1)

data_dev = dev[0] + "p4"
hash_dev = dev[0] + "p5"
store_dev = dev[0] + "p6"

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

command2 = "veritysetup --verbose format {0} {1}".format(data_dev, hash_dev)
print("Starting " + command2)
try:
	cmd2 = subprocess.check_output(command2.split(" ")).decode("utf-8")\
		                                             .splitlines()
except Exception:
	raise Exception("veritysetup command failed: {0}: {1}"
		            .format(command2, sys.exc_info()))
time.sleep(20)
if cmd2[len(cmd2) - 1] != "Command successful.":
	raise Exception("veritysetup failed to format")
salt_str = cmd2[len(cmd2) - 3]
root_hash_str = cmd2[len(cmd2) - 2]
print(salt_str)
print(root_hash_str)
salt = salt_str.replace('Salt:            	','')
root_hash = root_hash_str.replace('Root hash:      	','')
print(salt)
print(root_hash)


subprocess.check_output("mount {0} mnt".format(store_dev).split(" "))
print("Writing the root_hash to hash.txt in " + store_dev)
subprocess.check_output("touch mnt/hash.txt".split(" "))
try:
    outfile = open('mnt/hash.txt','w')
    outfile.write(root_hash)
    outfile.close()
except IOError:
    print("I/O error")

print("Writing the salt to salt.txt in " + store_dev)
subprocess.check_output("touch mnt/salt.txt".split(" "))
try:
    outfile = open('mnt/salt.txt','w')
    outfile.write(salt)
    outfile.close()
except IOError:
    print("I/O error")

subprocess.check_output("umount mnt".split(" "))
subprocess.check_output("rm -rf mnt".split(" "))

#command3 = "veritysetup --verbose verify {0} {1} {2}".format(data_dev, hash_dev, root_hash)
#
#print("Starting " + command3)
#try:
#        cmd3 = subprocess.check_output(command3.split(" ")).decode("utf-8")\
#                                                             .splitlines()
#except Exception:
#        raise Exception("veritysetup command failed: {0}: {1}"
#                            .format(command3, sys.exc_info()))
#time.sleep(20)
#if cmd3[len(cmd3) - 1] != "Command successful.":
#        raise Exception("veritysetup failed to verify")
#
#bsize_command = "sudo blockdev --getsz {0}".format(data_dev)
#
#print("Starting " + bsize_command)
#bsize = subprocess.check_output(bsize_command)
#print(bsize)
#arg = "0 " + bsize + " verity 1 " + data_dev + " " + hash_dev + " 4096 4096 52224 1 sha256 " + root_hash + " " + salt
#command5 = "dmsetup create name -r --table \"{0}\"".format(arg)
#print("Starting " + command5)
#try:
#        cmd5 = subprocess.check_output(command5.split(" ")).decode("utf-8")\
#                                                             .splitlines()
#except Exception:
#        raise Exception("dmsetup command failed: {0}: {1}"
#                            .format(command5, sys.exc_info()))
#time.sleep(20)
#
#
#command4 = "veritysetup --verbose create vroot {0} {1} {2}".format(data_dev, hash_dev, root_hash)
#print("Starting " + command4)
#try:
#        cmd4 = subprocess.check_output(command4.split(" ")).decode("utf-8")\
#                                                             .splitlines()
#except Exception:
#        raise Exception("veritysetup command failed: {0}: {1}"
#                            .format(command4, sys.exc_info()))
#time.sleep(20)
#if cmd4[len(cmd4) - 1] != "Command successful.":
#        raise Exception("veritysetup failed to create")

print("Completed successfully!")
