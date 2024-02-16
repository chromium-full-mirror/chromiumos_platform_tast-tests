#!/usr/bin/env python3
# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import argparse
import os
import pathlib
import sys

from guestlib import command
from guestlib import HostConnection


def test_setup(host_connection: HostConnection):
    """Runs sync and drop_caches on the guest and ask host to do the same.

    This function is supposed to be called before running a benchmark.
    """
    os.sync()
    with open("/proc/sys/vm/drop_caches", "w", encoding="utf-8") as f:
        f.write("3")

    # Signal host so the host drop caches.
    host_connection.signal_host_ready("")
    # Wait for host being ready to run the next test case
    host_connection.wait_for_host_signal()


def main():
    # We are running as pid 1.  Mount some necessary file systems.
    command(["mount", "-t", "proc", "proc", "/proc"])
    command(["mount", "-t", "sysfs", "sys", "/sys"])
    command(["mount", "-t", "tmpfs", "tmp", "/tmp"])
    command(["mount", "-t", "tmpfs", "run", "/run"])

    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--kind",
        choices=[
            "block",
            "block_lvm",
            "block_tpq",
            "virtiofs",
            "virtiofs_dax",
            "scsi",
            "pmem",
        ],
        required=True,
    )
    parser.add_argument(
        "--src", metavar="PATH", required=True, help="path to mount from"
    )
    parser.add_argument(
        "--mount-point",
        metavar="PATH",
        required=True,
        help="path to put directory for test data",
    )
    parser.add_argument(
        "--output",
        metavar="PATH",
        required=True,
        help="file name to store test results",
    )
    parser.add_argument(
        "--job-file",
        metavar="PATH",
        required=True,
        help=".job file to use for fio",
    )
    args = parser.parse_args()

    src = args.src
    mount_point = args.mount_point
    job_file = args.job_file
    output = args.output

    if args.kind in ("block", "block_lvm", "block_tpq", "scsi", "pmem"):
        p = pathlib.Path(src)
        if not p.is_block_device():
            print(f"{src} is not a block device")
            sys.exit(1)

        command(["/sbin/mkfs.ext4", src])

        # Same mount options as ARCVM's
        # /device/google/bertha/fstab.bertha.virtio_blk_data`
        # that cat affect performance.
        command(["mount", "-o", "rw,noatime,discard", src, mount_point])

    elif args.kind == "virtiofs":
        # Use the same mount options as `/device/google/bertha/fstab.bertha`
        # that cat affect theperformance.
        command(
            ["mount", "-t", "virtiofs", "-o", "rw,noatime", src, mount_point]
        )
    elif args.kind == "virtiofs_dax":
        command(
            [
                "mount",
                "-t",
                "virtiofs",
                "-o",
                "rw,noatime,dax",
                src,
                mount_point,
            ]
        )
    else:
        print(f"{args.kind} is not supported kind")
        assert False

    host_connection = HostConnection()

    # Create a 1G size FIO test file and fill with random data
    command(
        [
            "dd",
            "if=/dev/urandom",
            f"of={mount_point}/fio_file",
            "bs=4k",
            "count=128k",
        ]
    )

    # Notify the host that the guest created a test file.
    # Host should drop caches.
    test_setup(host_connection)

    # Run the FIO test
    # Test will either finish performing IO size that equals to fio_file size
    # or stops after executing 3 minutes
    command(
        [
            "/usr/local/bin/fio",
            f"--directory={mount_point}",
            "--runtime=3m",
            "--time_based=0",
            "--blocksize=4K",
            f"--output={output}",
            "--output-format=json",
            "--filename=fio_file",
            job_file,
        ]
    )

    host_connection.signal_host_complete()


if __name__ == "__main__":
    main()

    # Avoid calling poweroff if this script doesn't run as an init process for debug purpose.
    if os.getpid() == 1:
        os.system("poweroff -f")
