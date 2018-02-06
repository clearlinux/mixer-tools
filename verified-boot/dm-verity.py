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

src = dev[0] + "p4"
dest = dev[0] + "p5"

command2 = "veritysetup --verbose format {0} {1}".format(src, dest)
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

#command3 = "veritysetup --verbose verify {0} {1} {2}".format(src, dest, root_hash)

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

#bsize_command = "sudo blockdev --getsz {0}".format(src)

#print("Starting " + bsize_command)
#bsize = subprocess.check_output(bsize_command)
#print(bsize)
#arg = "0 " + bsize + " verity 1 " + src + " " + dest + " 4096 4096 52224 1 sha256 " + root_hash + " " + salt
#command5 = "dmsetup create name -r --table \"{0}\"".format(arg)
#print("Starting " + command5)
#try:
#        cmd5 = subprocess.check_output(command5.split(" ")).decode("utf-8")\
#                                                             .splitlines()
#except Exception:
#        raise Exception("dmsetup command failed: {0}: {1}"
#                            .format(command5, sys.exc_info()))
#time.sleep(20)


#command4 = "veritysetup --verbose create vroot {0} {1} {2}".format(src, dest, root_hash)
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
