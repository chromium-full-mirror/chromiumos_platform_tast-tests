# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""The script used to run GeekbenchCUJ with external binary

geekbench.py will take in a valid geekbench email and license
and run Geekbench5CUJ or Geekbench6CUJ using either the default
x86_64 binary or a custom binary passed in.

Required positional argumentss:
1. Geekbench license email
2. Geekbench license key
3. DUT address

Optional flags:
1. --binary (-b) Custom binary path
2. --gb-version -v Geekbench version. Default 5.
3. --env -e Execution environment; native and/or crostini. Default native.
"""
import argparse
import os
import subprocess
import sys
from pathlib import Path
from typing import Optional

VALID_ENV = ["native", "crostini", "both"]
THIS_FILE = Path(__file__).resolve()
CHROMEOS_CHECKOUT_PATH = THIS_FILE.parent.parent.parent.parent.parent
BINARY_DEST= (
    CHROMEOS_CHECKOUT_PATH /
    "src/platform/tast-tests/src/go.chromium.org/"
    "tast-tests/cros/local/bundles/cros/ui/data/geekbench_custom"
    )

def parse_argument(argv) -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("email", type=str, help="Geekbench email")
    parser.add_argument("key", type=str, help="Geekbench key")
    parser.add_argument("dut", type=str, help="DUT")
    parser.add_argument("--binary", "-b", type = str,
                        help="Geekbench binary path")
    parser.add_argument("--gb-version", "-v", type=str,
                        help="Geekbench version", default="5")
    parser.add_argument("--env", "-e", type=str,
                        help="Environment to run in: [native, crostini, both]",
                        default="native")

    return parser.parse_args(argv)

def construct_command(args) -> str:
    if str(args.gb_version) not in ["5", "6"]:
        raise ValueError(
            "Invalid Geekbench version; must be 5 or 6"
        )

    tast_command = ("cros_sdk tast run -var=geekbench.email={email}"
                    " -var=geekbench.key={key} {dut}")
    env = str(args.env)

    if env not in VALID_ENV:
        raise ValueError(
            "Invalid environment; must be of [native, crostini, both]"
            )
    elif env in ("native", "both"):
        tast_command += " ui.Geekbench{version}CUJ"
        if args.binary is not None:
            tast_command += ".custom"
    elif env in ("crostini", "both"):
        tast_command += "ui.Geekbench{version}CUJ.crostini"
        if args.binary is not None:
            tast_command += "_custom"
    tast_command = tast_command.format(dut=args.dut, email=args.email,
                        key=args.key, version=args.gb_version)

    return tast_command

def copy_binary(binary_src) -> None:
    """Copy binary from binary_src to the destination in chromiumos checkout.

    Raise an Exception if the copy command failed.
    """
    cp_cmd = "cp {src} {dst}"
    if os.system(cp_cmd.format(src=binary_src, dst=BINARY_DEST)) != 0:
        raise Exception("failed to copy binary")

def run_test(tast_command, args) -> None:
    """Run the tast command. Delete binary after it returns."""
    try:
        subprocess.run(
            tast_command.split(),
            cwd=CHROMEOS_CHECKOUT_PATH,
            check=True,
        )
    except subprocess.CalledProcessError:
        print("tast command failed")
    finally:
        # If a binary is copied, delete it after tast command returns.
        if args.binary is not None:
            rm_cmd = "rm {path}"
            os.system(rm_cmd.format(path=BINARY_DEST))


def main(argv) -> Optional[int]:
    args=parse_argument(argv)

    tast_command = construct_command(args)

    if args.binary is not None:
        copy_binary(args.binary)

    run_test(tast_command, args)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
