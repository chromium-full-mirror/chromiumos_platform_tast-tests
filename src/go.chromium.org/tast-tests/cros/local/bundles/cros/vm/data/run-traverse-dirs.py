#!/usr/bin/env python3
# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This script is meant to be run as PID 1 inside a VM.

import argparse
import os

from guestlib import command
from guestlib import HostConnection


def main():
    """Mount a given virtual storage device and measure performance of file operations there."""
    parser = argparse.ArgumentParser(
        description="Run test accessing many files"
    )
    parser.add_argument(
        "--kind",
        choices=["pmem-ext2", "pmem-ext2-dax", "virtiofs"],
        required=True,
    )
    parser.add_argument(
        "--mount-src", metavar="PATH", required=True, help="path to mount from"
    )
    parser.add_argument(
        "--working-dir",
        metavar="PATH",
        required=True,
        help="path to put directory for test data",
    )
    args = parser.parse_args()

    # Create '${working_dir}/mount' where we can mount a virtio-{blk, fs} device.
    mount_dir = os.path.join(args.working_dir, "mount")
    os.mkdir(mount_dir)

    # Mount guest's procfs on `/proc` to overload the host's procfs shared via virtiofs.
    command(["mount", "-t", "proc", "proc", "/proc"])

    if args.kind == "pmem-ext2" or args.kind == "pmem-ext2-dax":
        options = "rw,noatime,nosuid,nodev"
        if args.kind == "pmem-ext2-dax":
            options += ",dax"
        # Use the same mount options as `/device/google/bertha/fstab.bertha`
        command(
            [
                "mount",
                "-t",
                "ext2",
                "-o",
                options,
                args.mount_src,
                mount_dir,
            ]
        )
    elif args.kind == "virtiofs":
        # Use the same mount options as `/device/google/bertha/fstab.bertha`
        command(
            [
                "mount",
                "-t",
                "virtiofs",
                "-o",
                "rw,noatime,nosuid,nodev",
                args.mount_src,
                mount_dir,
            ]
        )
    else:
        assert False

    conn = HostConnection()

    conn.signal_host_ready("Traverse")
    conn.wait_for_host_signal()

    # Read all the file under `mount_dir` and count
    cnt = 0
    for root, dirs, files in os.walk(mount_dir):
        cnt += len(dirs) + len(files)
        # Ignore 'lost+found' created by crosvm's ext2 logic
        if "lost+found" in dirs:
            cnt -= 1
        for name in files:
            path = os.path.join(root, name)
            if os.path.islink(path):
                continue
            with open(path, "rb") as f:
                _ = f.read()

    conn.signal_host_message(str(cnt))


if __name__ == "__main__":
    main()

    # Avoid calling poweroff if this script doesn't run as an init process for debug purpose.
    if os.getpid() == 1:
        os.system("poweroff -f")
