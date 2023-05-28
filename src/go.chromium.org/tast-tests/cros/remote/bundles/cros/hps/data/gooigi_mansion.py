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
    import digitalio
except RuntimeError:
    sys.exit("Board not connected")

import gooigi_dmatrix
import gooigi_PWM_PCA9685

"""Servo motor duty cycle must be calibrated on a brand/model/unit basis. These constants represent the dutycycle required for the installed motors to move between 0 and 180 degrees, they may need to be changed if the motors are swapped out."""
SERVO_0_DEG_PERCENT = 0.02
SERVO_180_DEG_PERCENT = 0.13

# Time allowed to record the approximate speed in which DMD is being refreshed to calibrate a PWM frequency that reduces flicker.
CALIBRATION_TIME = 2


class Figure(object):
    """Figure class handles control of figurine.

    Attributes:
        direction_servo: Servo used to determine direction figurine is facing.
        visibility_servo: Servo used to determine visibility of figurine.
        hidden_angle: Visibility servo angle in which figurine is most out of frame by lying horizontally.
        vert_angle: Visibility servo angle in which figureine is upright.
        hps_angle: Direction servo angle in which figurine is facing the hps unit.
    """

    def __init__(
        self,
        direction_servo: gooigi_PWM_PCA9685.Servo,
        visibility_servo: gooigi_PWM_PCA9685.Servo,
        hidden_angle=100,
        vert_angle=180,
        hps_angle=90,
    ):
        self._direction_servo = direction_servo
        self._visibility_servo = visibility_servo
        self._hidden_angle = hidden_angle
        self._hps_angle = hps_angle
        self._vert_angle = vert_angle

    def display(self):
        """Position figurine vertically."""
        self._visibility_servo.set_angle(self._vert_angle)
        self.face_forward()

    def hide(self):
        """Position figurine horizontally."""
        self.face_forward()
        self._visibility_servo.set_angle(self._hidden_angle)

    def face_forward(self):
        """Face figurine forward."""
        self._direction_servo.set_angle(self._hps_angle)

    def face_angle(self, angle=None):
        """Face figurine towards specific angle."""
        if angle is None:
            angle = self._hps_angle
        self._direction_servo.set_angle(angle)

    def disable(self):
        """Disable positioning servo motors."""
        self._direction_servo.set_off()
        self._visibility_servo.set_off()

    def vis_servo_calibration_helper(self):
        """Helper function that oscillates between hidden and in view angles to help calibrate figuring attachment."""
        while True:
            print("show:", self._vert_angle)
            self.display()
            time.sleep(3)
            print("hide: ", self._hidden_angle)
            self.hide()
            time.sleep(3)


class Enclosure(object):
    """Enclosure class handles controlling the Gooigi camerabox testing conditions.

    Attributes:
        servo_enable_pin: Pin to control signal to gate of N-channel mosfet.
        LED_frequency: Frequency used for LED PWM signal.
        servo_frequency: Frequency used for servo PWM signal.
        PCA9685: Object to control frequency and channels of PWM output.
        dmd_brightness: Object to control brightness of DMD panels.
        dmd: Object to control dot matrix display of enclosure.
        main_figure: Object to control the main figurine in the enclosure.
        secondary_figure: Object to control the secondary figurine in the enclosure.
    """

    def __init__(self):
        self._servo_enable_pin = digitalio.DigitalInOut(board.C7)
        self._servo_enable_pin.direction = digitalio.Direction.OUTPUT
        self._servo_enable_pin.value = False

        self._led_frequency = 1250
        self._servo_frequency = 50

        self._PCA9685 = self.initialise_pca9685()
        self._dmd_brightness = self._PCA9685.initialise_led(0, 100)
        self.dmd = self.initialise_dmd(self._dmd_brightness)

        self._main_figure = Figure(
            self._PCA9685.initialise_servo(
                14, SERVO_0_DEG_PERCENT, SERVO_180_DEG_PERCENT
            ),
            self._PCA9685.initialise_servo(
                15, SERVO_0_DEG_PERCENT, SERVO_180_DEG_PERCENT
            ),
            hidden_angle=110,
            hps_angle=80,
        )
        self._secondary_figure = Figure(
            self._PCA9685.initialise_servo(
                13, SERVO_0_DEG_PERCENT, SERVO_180_DEG_PERCENT
            ),
            self._PCA9685.initialise_servo(
                12, SERVO_0_DEG_PERCENT, SERVO_180_DEG_PERCENT
            ),
            hidden_angle=170,
            vert_angle=110,
            hps_angle=80,
        )

    def initialise_dmd(self, oe):
        """Initiates dot matrix display object for controlling the enclosure's lighting.

        Args:
            oe: Output enable LED pwm control object.
        """
        return gooigi_dmatrix.DotMatrixDisplay(
            1, 4, board.C0, board.C1, board.D5, board.D6, board.D7, oe
        )

    def initialise_pca9685(self):
        """Initiate PCA9685 for PWM usage."""
        i2c_bus = busio.I2C(board.SCL, board.SDA)
        return gooigi_PWM_PCA9685.Gooigi_PCA9685(i2c_bus, self._servo_frequency)

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

        Because of the limitations of the LED matrix in use, the maximum PWM frequency achievable is not sufficient to eliminate flicker in the panels.
        By calibrating and finding the current running speed of the matrix row swapping operation, the flicker can be minimised.
        """
        start_time = time.time()
        counter = 0
        while time.time() < start_time + CALIBRATION_TIME:
            self.dmd.swap_active_rows()
            counter += 1

        swaps_per_sec = counter / (time.time() - start_time)
        self._led_frequency = swaps_per_sec // 2

    def enable_servos(self):
        """Set PWM frequency and enable N-channel mosfet that provides power to servos."""
        self._PCA9685.change_frequency(self._servo_frequency)
        self._servo_enable_pin.value = True

    def disable_servos(self):
        """Disable N-channel mosfet that provides power to servos, set servo signals to low, and update PWM frequency for usage with LEDs."""
        self._servo_enable_pin.value = False
        self._main_figure.disable()
        self._secondary_figure.disable()
        self._PCA9685.change_frequency(self._led_frequency)

    def _set_figure(self, figure: Figure, visible, angle):
        if visible:
            figure.display()
            if angle is None:
                figure.face_forward()
            else:
                figure.face_angle(angle)
        else:
            figure.hide()

    def set_figures(
        self,
        main_fig_visible,
        secondary_fig_visible,
        main_fig_angle=None,
        secondary_fig_angle=None,
    ):
        """Update positions of enclosure figurines.

        Args:
            main_fig_visible: Boolean to determine if main figure should be visible.
            secondary_fig_visible: Boolean to determine if secondary figure should be visible.
            main_fig_angle: Angle that main figure should be facing.
            secondary_fig_angle: Angle that secondary figure should be facing.
        """
        self._set_figure(self._main_figure, main_fig_visible, main_fig_angle)
        self._set_figure(
            self._secondary_figure, secondary_fig_visible, secondary_fig_angle
        )


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
    parser.add_argument(
        "-m",
        "--main_subject",
        help="Moves main figure into frame",
        action="store_true",
    )
    parser.add_argument(
        "-s",
        "--secondary_subject",
        help="Moves secondary figure into frame",
        action="store_true",
    )

    return parser.parse_args()


def main():
    print("Python script launched.")
    args = parse_args()
    enc = Enclosure()
    print("Enclosure initiated.")

    enc.enable_servos()
    enc.set_figures(args.main_subject, args.secondary_subject, None, None)
    print("Figures in position.")

    enc.calibrate_display()
    enc.set_display_brightness(args.panel_power)
    print("Screen calibration complete.")

    """
    Servo control is required to operate at a frequency of 50Hz, outside of this the behaviour is unexpected.
    However since the same PWM driver is used to control the LED panel dimming, the higher the frequency the better to reduce flickering.
    To use different frequencies without damaging the servos, power is cut to the servo motors and their signal is set to 0% dutycycle before swapping the frequency to a higher option.
    """
    enc.disable_servos()

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
