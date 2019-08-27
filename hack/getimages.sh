#!/bin/bash

dir=images
mkdir -p $dir; pushd $dir

base_url=http://dl.bintray.com/dougm/ttylinux
ttylinux="ttylinux-pc_i486-16.1"
file="${ttylinux}.ova"

#download the ova.
#TODO: mirror ova content internally.
curl -L -O $base_url/$file

# extract ova so we can also use the .vmdk and .ovf files directly
tar -xvf ${ttylinux}.ova

popd
