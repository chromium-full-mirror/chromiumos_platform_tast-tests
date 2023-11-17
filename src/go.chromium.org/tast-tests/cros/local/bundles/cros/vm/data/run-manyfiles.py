#!/usr/bin/env python3
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This script is meant to be run as PID 1 inside a VM.

import argparse
import json
import os
import shutil
import subprocess
import timeit
from typing import Generator, List

NUM_FILES = 10000
NUM_DIRS = 10000

HOST_CONNECTION = None
OUT_DIR = ""

def command(cmd: List[str]):
    """Runs a shell command."""
    joined = " ".join(cmd)
    print(f'RUN: {joined}')
    subprocess.run(cmd, check=True)

def test_setup(name: str):
    """Runs sync and drop_caches on the guest and ask host to do the same.

    This function is supposed to be called before running a benchmark.
    """
    os.sync()
    with open('/proc/sys/vm/drop_caches', 'w') as f:
        f.write('3')

    # Signal host so the host drop caches.
    HOST_CONNECTION.signal_host_ready(name)
    # Wait for host being ready to run the next test case
    HOST_CONNECTION.wait_for_host_signal()

def test_setup_without_drop_caches(name: str):
    """Notify the host that we are ready to run the next test and not to drop
    caches.

    This function is supposed to be called before running a benchmark.
    """
    HOST_CONNECTION.signal_host_ready_no_drop(name)
    HOST_CONNECTION.wait_for_host_signal()

def measure(name: str, code: str, drop_caches: bool) -> float:
    """Measures how many milliseconds it takes to run the given code snippet."""
    print(f'Measure "{name}"')
    if drop_caches:
        setup = lambda: test_setup(name)
    else:
        setup = lambda: test_setup_without_drop_caches(name)
    sec = timeit.timeit(code, setup=setup, number=1, globals=globals())
    return sec * 1000 # Converts seconds to milliseconds


def file_path_generator() -> Generator[str, None, None]:
    """Generates file names in the following directory structure, which mimics a Unity-based Android application's cache directory.

    ${OUT_DIR}/
    |-- file-0000.txt
    |-- file-0001.txt
    ...
    |-- file-{NUM_FILES}.txt
    `-- dir-0000/
    |   `-- a.txt
    `-- dir-0001/
    |   `-- a.txt
    ...
    `-- dir-{NUM_DIRS}/
      `-- a.txt
    """

    for i in range(NUM_FILES):
        yield f'{OUT_DIR}/file-{i:04}.txt'

    for i in range(NUM_DIRS):
        yield f'{OUT_DIR}/dir-{i:04}/a.txt'


def dir_path_generator() -> Generator[str, None, None]:
    """Generates directory names in the directory structure defined in `file_path_generator()`"""
    for i in range(NUM_DIRS):
        yield f'{OUT_DIR}/dir-{i:04}'


def test_create():
    """Creates many files"""
    d_gen = dir_path_generator()
    for d in d_gen:
        os.mkdir(d)
    f_gen = file_path_generator()
    for f in f_gen:
        # Use os.open() instead of the builtin 'open()' to avoid extra syscalls.
        fd = os.open(f, os.O_CREAT | os.O_WRONLY)
        os.close(fd)

    command(['sync'])


def test_open():
    """Opens many files"""
    f_gen = file_path_generator()
    for f in f_gen:
        # Use os.open() to avoid extra syscalls.
        fd = os.open(f, os.O_RDONLY)
        os.close(fd)

def test_open_non_existent():
    """Tries to open many non-existing files"""
    f_gen = file_path_generator()
    for f in f_gen:
        fake = f'{f}-nonexistent'
        try:
            _ = os.open(fake, os.O_RDONLY)
            assert False
        except FileNotFoundError:
            pass


def test_remove():
    """Removes a directory with many files"""
    shutil.rmtree(OUT_DIR)
    command(['sync'])


class HostConnection:
    """Serial connection to the host"""

    def __init__(self):
        self.input = open('/dev/ttyS1')
        self.output = open('/dev/ttyS1', 'w')

    def __del__(self):
        self.input.close()
        self.output.close()

    def signal_host_ready(self, name: str):
        """Sends a string 'READY' to the host"""
        self.output.write(f'READY:{name}\n')
        self.output.flush()

    def signal_host_ready_no_drop(self, name: str):
        """Sends a string 'READY_NO_DROP' to the host for a test case that does
        not drop caches.
        """
        self.output.write(f'READY_NO_DROP:{name}\n')
        self.output.flush()

    def signal_host_end(self):
        """Sends a string 'END' to the host
        This should be sent when each test case is completed.
        """
        self.output.write('END\n')
        self.output.flush()

    def signal_host_complete(self):
        """Sends a string 'COMPLETE' to the host
        This should be sent when all the test cases are completed.
        """
        self.output.write('COMPLETE\n')
        self.output.flush()

    def wait_for_host_signal(self):
        """Receives a string 'RUN' from the host"""
        line = self.input.readline()
        assert line.strip() == "RUN"


def main():
    """Mount a given virtual storage device and measure performance of file operations there."""
    parser = argparse.ArgumentParser(
        description="Run test accessing many files")
    parser.add_argument('--kind', choices=['block', 'virtiofs'], required=True)
    parser.add_argument('--mount-src', metavar='PATH',
                        required=True, help='path to mount from')
    parser.add_argument('--working-dir', metavar='PATH',
                        required=True, help='path to put directory for test data')
    parser.add_argument('--output-json', metavar='PATH',
                        required=True, help='file name to store test results')
    args = parser.parse_args()

    # Create '${working_dir}/mount' where we can mount a virtio-{blk, fs} device.
    mount_dir = os.path.join(args.working_dir, 'mount')
    os.mkdir(mount_dir)

    # Mount guest's procfs on `/proc` to overload the host's procfs shared via virtiofs.
    command(['mount', '-t', 'proc', 'proc', '/proc'])

    if args.kind == 'block':
        # Use mkfs flags that are close to the one used for ARCVM's /data/.
        # https://source.corp.google.com/rvc-arc/vendor/google_arc/services/arc-mkfs-blk-data/main.cpp;l=70;rcl=4968db28c8d4466aefda34adcec9a81b30dc3e61
        # TODO(b/275507715): Add 'casefold,project,quota' to `-O` and pass
        # '-Equotatype=usrquota:grpquota:prjquota'.
        # These flags cannot be enabled because we're using Crostini guest kernels, which have
        # different sets of kernel configs.
        # We need to either (1) use ARCVM guest kernel in this test or (2) enable CONFIG_UNICODE
        # for 'casefold' and CONFIG_QUOTA for other flags.
        command(['/sbin/mkfs.ext4',
                 '-b', '4096',
                 f'-Overity',
                 args.mount_src])

        # Same mount option as ARCVM's /device/google/bertha/fstab.bertha.virtio_blk_data.
        command(['mount', '-t', 'ext4', '-o', 'rw,noatime,nosuid,nodev,discard,resgid=1065', args.mount_src, mount_dir])
    elif args.kind == 'virtiofs':
        # Use the same mount options as `/device/google/bertha/fstab.bertha`
        command(['mount', '-t', 'virtiofs', '-o', 'rw,noatime,nosuid,nodev,noexec', args.mount_src, mount_dir])
    else:
        assert False

    # Establish a serial connection to the host
    global HOST_CONNECTION
    HOST_CONNECTION = HostConnection()
    # Create a directory 'many-files' on the mounted disk.
    global OUT_DIR
    OUT_DIR = os.path.join(mount_dir, 'many-files')
    os.mkdir(OUT_DIR)

    test_cases = [
        # (test name, function name, whether caches are dropped before the test)
        # The test cases '*_twice' are ones that run without dropping caches to
        # evaluate the effects of caches.
        ('create', 'create', True),
        ('open', 'open', True),
        ('open_second', 'open', False),
        ('open_non_existent', 'open_non_existent', True),
        ('open_non_existent_second', 'open_non_existent', False),
        ('remove', 'remove', True),
    ]
    results = {}

    for (case, func, drop_caches) in test_cases:
        results[case] = measure(case, f'test_{func}()', drop_caches)
        HOST_CONNECTION.signal_host_end()

    HOST_CONNECTION.signal_host_complete()

    with open(args.output_json, 'w', encoding='utf-8') as f:
        json.dump(results, f)


if __name__ == "__main__":
    main()

    # Avoid calling poweroff if this script doesn't run as an init process for debug purpose.
    if os.getpid() == 1:
        os.system("poweroff -f")
