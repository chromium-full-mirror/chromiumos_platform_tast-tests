#!/bin/bash
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This script is meant to be run as PID 1 inside a VM.
set -ex

die() {
    echo "$1"
    exit 1
}

main() {
    [[ "$#" == 2 ]] || \
       die "Usage $(basename "$0") ADDR GATEWAY"

    local addr="$1"
    local gateway="$2"

    # We are running as pid 1.  Mount some necessary file systems.
    mount -t proc proc /proc
    mount -t sysfs sys /sys
    mount -t tmpfs tmp /tmp
    mount -t tmpfs run /run

    net_dev=eth0
    ip a add "${addr}"/30 dev "${net_dev}"
    ip link set "${net_dev}" up
    ip route add default via "${gateway}"
    # Print IP info for debug
    ip a
    ip route show all

    iperf3 -V -J -s -p 1234 -1
}

main "$@"
exec poweroff -f
