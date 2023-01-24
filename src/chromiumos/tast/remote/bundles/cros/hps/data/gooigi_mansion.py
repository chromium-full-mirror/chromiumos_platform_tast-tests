# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

import os
import argparse
import time
import sys

# Export environment variable to correctly import the board package as this variable tells the package what hardware it should use
os.environ["BLINKA_FT232H"] = "1"

try:
    import board
except RuntimeError:
    sys.exit("Board not connected")

import gooigi_dmatrix


class Enclosure(object):
    """Enclosure class handles controlling the Gooigi camerabox testing conditions.

    Attributes:
        dmd: Dot matrix display used in enclosure.
    """

    def __init__(self):
        self.dmd = self.initialise_dmd()

    def initialise_dmd(self):
        """Initiates dot matrix display object for controlling the enclosure's lighting."""
        return gooigi_dmatrix.DotMatrixDisplay(
            1, 4, board.C0, board.C1, board.D5, board.D6, board.D7, board.D4
        )

    def enable_back_panel(self):
        """Turns all LEDs on the panel to the rear of the subjects on."""
        self.dmd.set_panel(1, True)
        self.dmd.set_panel(2, True)

    def enable_left_panel(self):
        """Turns all LEDs on the panel to the left of the subjects on."""
        self.dmd.set_panel(3, True)

    def enable_right_panel(self):
        """Turns all LEDs on the panel to the right of the subjects on."""
        self.dmd.set_panel(0, True)


def parse_args():
    parser = argparse.ArgumentParser(
        description="Enclosure control program for HPS high brightness testing suite."
    )

    parser.add_argument(
        "-b",
        "--back_panel",
        help="Turns on the back LED panel",
        action="store_true",
    )
    parser.add_argument(
        "-r",
        "--right_panel",
        help="Turns on the right LED panel",
        action="store_true",
    )
    parser.add_argument(
        "-l",
        "--left_panel",
        help="Turns on the left LED panel",
        action="store_true",
    )
    parser.add_argument(
        "-d",
        "--duration",
        help="Duration of test in seconds",
        type=int,
        default=1,
    )

    return parser.parse_args()


def main():
    print("Python script launched.")
    args = parse_args()
    enc = Enclosure()
    print("Enclosure initiated.")

    if args.back_panel:
        enc.enable_back_panel()
    if args.left_panel:
        enc.enable_left_panel()
    if args.right_panel:
        enc.enable_right_panel()
    enc.dmd.scan_display()
    print("Screen configuration updated.")

    start_time = time.time()
    while time.time() < start_time + args.duration:
        enc.dmd.swap_active_rows()
    print("Testing complete.")

    enc.dmd.set_screen(False)
    enc.dmd.scan_display()

    print("Exiting.")


if __name__ == "__main__":
    main()
