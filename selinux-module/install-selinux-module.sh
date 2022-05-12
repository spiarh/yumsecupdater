#!/usr/bin/env bash

set -euxo pipefail

echo ">>> copy SELinux module to host"
cp /selinux-module/yumsecupdater.pp /host/tmp/yumsecupdater.pp

echo ">>> install SELinux module"
/usr/bin/nsenter -m/proc/1/ns/mnt -- semodule -i /tmp/yumsecupdater.pp

echo ">>> remove SELinux module tmp file"
rm /host/tmp/yumsecupdater.pp
