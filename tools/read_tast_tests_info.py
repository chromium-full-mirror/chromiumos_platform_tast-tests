#!/usr/bin/env vpython3
# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""
Parse output of `tast list -json $DUT <pattern>` and print out selected fields
of tests. One test per line. Names  of fields could be found in
`resultjson.Test`.

Usage:
  tast list -json $DUT <pattern> | \
          read_tast_tests_info.py -field field_name1,field_name2

"""

import argparse
import json
import sys
from typing import Optional


def is_valid_field(name) -> bool:
    ALLOWED = [
        "name",
        "pkg",
        "desc",
        "contacts",
        "attr",
        "searchFlags,omitempty",
        "data",
        "vars",
        "varDeps",
        "softwareDeps",
        "serviceDeps",
        "fixture",
        "timeout",
        "bundle",
        "testBedDeps",
        "requirements",
        "bugComponent",
        "lacros_status",
    ]
    return name in ALLOWED


def print_test_info(test_info, fields, delimiter):
    """Print the fields of the given list of tests."""
    for test in test_info:
        first = True
        for f in fields:
            if not first:
                print(delimiter, end="")
            if f in test:
                print(test[f], end="")
            else:
                print("<empty>", end="")

            first = False
        print()


def parse_arguments(argv) -> argparse.Namespace:
    """Parse arguments and return the argparse.Namespace object"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--field",
        type=str,
        default="name",
        help=(
            "Comma separated list of tast test field names. The names could be"
            " found in http://shortn/_U5pND019CG"
        ),
    )
    parser.add_argument(
        "--delimiter",
        type=str,
        default=" ",
        help=(
            "Delimiter used to separate printed out fields."
        ),
    )
    return parser.parse_args(argv)


def main(argv) -> Optional[int]:
    opts = parse_arguments(argv)

    fields = [f.strip() for f in opts.field.split(",")]
    for f in fields:
        if not is_valid_field(f):
            print("Invalid field: ", f)
            return

    print_test_info(json.load(sys.stdin), fields, opts.delimiter)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
