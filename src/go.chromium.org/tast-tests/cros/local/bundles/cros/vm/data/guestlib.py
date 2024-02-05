# Copyright 2024 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import subprocess
from typing import List


def command(cmd: List[str]):
    """Runs a shell command."""
    joined = " ".join(cmd)
    print(f"RUN: {joined}")
    subprocess.run(cmd, check=True)


class HostConnection:
    """Serial connection to the host"""

    def __init__(self):
        self.input = open("/dev/ttyS1", encoding="utf-8")
        self.output = open("/dev/ttyS1", "w", buffering=1, encoding="utf-8")

    def __del__(self):
        self.input.close()
        self.output.close()

    def signal_host_ready(self, name: str):
        """Sends a string 'READY' to the host"""
        self.output.write(f"READY:{name}\n")

    def signal_host_ready_no_drop(self, name: str):
        """Sends a string 'READY_NO_DROP' to the host for a test case that does
        not drop caches.
        """
        self.output.write(f"READY_NO_DROP:{name}\n")

    def signal_host_end(self):
        """Sends a string 'END' to the host
        This should be sent when each test case is completed.
        """
        self.output.write("END\n")

    def signal_host_complete(self):
        """Sends a string 'COMPLETE' to the host
        This should be sent when all the test cases are completed.
        """
        self.output.write("COMPLETE\n")

    def wait_for_host_signal(self):
        """Receives a string 'RUN' from the host"""
        line = self.input.readline()
        assert line.strip() == "RUN"
