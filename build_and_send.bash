#!/bin/bash

tmpDir=$(mktemp -d)
currDir=$(pwd)
cp -rp * $tmpDir/

cd $tmpDir
zip dhcp-src.zip -qr *

docker run -i openwhisk/action-golang-v1.15 -compile main <dhcp-src.zip >dhcp-bin.zip

# sshpass -p #### scp dhcp-bin.zip ####:#####

cd $currDir

if [[ ! -z $tmpDir && $tmpDir != "/" ]]; then
	rm -rf $tmpDir
fi
