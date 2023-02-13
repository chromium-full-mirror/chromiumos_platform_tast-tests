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
    import busio
except RuntimeError:
    sys.exit("Board not connected")

import gooigi_dmatrix
import gooigi_PWM_PCA9685

# Time allowed to record the approximate speed in which DMD is being refreshed to calibrate a PWM frequency that reduces flicker.
CALIBRATION_TIME = 2


class Enclosure(object):
    """Enclosure class handles controlling the Gooigi camerabox testing conditions.

    Attributes:
        dmd: Dot matrix display used in enclosure.
    """

    def __init__(self):
        self._led_frequency = 1250
        self._PCA9685 = self.initialise_pca9685()
        self._dmd_brightness = self._PCA9685.initialise_led(0, 100)
        self.dmd = self.initialise_dmd(self._dmd_brightness)

    def initialise_dmd(self, oe):
        """Initiates dot matrix display object for controlling the enclosure's lighting."""
        return gooigi_dmatrix.DotMatrixDisplay(
            1, 4, board.C0, board.C1, board.D5, board.D6, board.D7, oe
        )

    def initialise_pca9685(self):
        i2c_bus = busio.I2C(board.SCL, board.SDA)
        return gooigi_PWM_PCA9685.Gooigi_PCA9685(i2c_bus, self._led_frequency)

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

    def set_display_brightness(self, percentage):
        """Set brightness of dot matrix display.

        Args:
            percentage: Brightness as a percentage from 0 (LEDs are off) to 100 (LEDs are constantly on). The brightness represents the percentage the LEDs are on during the PWM duty cycle.
        """
        assert percentage >= 0 and percentage <= 100
        self._dmd_brightness.set_brightness(percentage)

    def calibrate_display(self):
        """Calibrate frequency of PWM to reduce flicker.

        Because of the limitations of the LED matrix in use, the maximum PWM frequency achievable is not sufficient to eliminate flicker in the panels. By calibrating and finding the current running speed of the matrix row swapping operation, the flicker can be minimised.
        """
        start_time = time.time()
        counter = 0
        while time.time() < start_time + CALIBRATION_TIME:
            self.dmd.swap_active_rows()
            counter += 1

        swaps_per_sec = counter / (time.time() - start_time)
        self._led_frequency = swaps_per_sec // 2


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
    parser.add_argument(
        "-p",
        "--panel_power",
        help="Determines strength of LED panels",
        type=int,
        default=100,
    )

    return parser.parse_args()


def main():
    print("Python script launched.")
    args = parse_args()
    enc = Enclosure()
    print("Enclosure initiated.")

    enc.calibrate_display()
    enc.set_display_brightness(args.panel_power)
    print("Screen calibration complete.")

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
