#!/usr/bin/env vpython3
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Print out test timeout info for a given DUT and test pattern.

Run `tast list` against the given DUT ant test pattern and output the
test timeout info.

Usage:
  print_test_timeout.py $DUT <pattern>
"""

from pathlib import Path
import subprocess
import sys
from typing import Optional


def to_hms(nano) -> str:
    """Convert the given duration in nanoseconds to [hour] + minute + seconds format."""
    hour = int(nano / 1000000000 / 3600)
    minute = int(nano / 1000000000 % 3600 / 60)
    seconds = int(nano / 1000000000 % 60)

    optional_hour = ""
    if hour > 0:
        optional_hour += str(hour) + "h"
    return optional_hour + str(minute) + "m" + str(seconds) + "s"


def print_test_timeout(lines):
    total = 0
    for l in lines:
        if not l:
            continue

        fields = [f.strip() for f in l.split()]
        nano = int(fields[1])
        total += nano
        print(fields[0], " ", to_hms(nano))

    print("Total ", to_hms(total))


def main(argv) -> Optional[int]:
    dut = argv[0]
    pattern = argv[1]

    # Assumes `read_tast_tests_info.py` is in the same dir of this script.
    read_tast_tests_info = (
        Path(__file__).resolve().parent / "read_tast_tests_info.py"
    )

    # Public tests.
    cmd = (
        "tast list -json "
        + dut
        + " '"
        + pattern
        + "' | "
        + read_tast_tests_info.as_posix()
        + " --field name,timeout"
    )
    out = subprocess.getoutput(cmd)

    # Private tests.
    cmd = (
        "tast list -json -buildbundle=crosint "
        + dut
        + " '"
        + pattern
        + "' | "
        + read_tast_tests_info.as_posix()
        + " --field name,timeout"
    )
    out += "\n" + subprocess.getoutput(cmd)

    print_test_timeout([l.strip() for l in out.split("\n")])


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
