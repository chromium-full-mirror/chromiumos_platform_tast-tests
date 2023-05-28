# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

from adafruit_pca9685 import PCA9685

MAX_DUTY_CYCLE = 0xFFFF


class Gooigi_PCA9685(object):
    """PCA9685 class handles creating different PWM signals from PCA9685 chip.

    Attributes:
        i2c_bus: i2c protocol bus to use to communicate with the PCA9685.
        frequency: Initial frequency of PWM signal.
        pca: PCA9685 object used to control PWM.
        channels: List used to store configuration of all the channels.
    """

    def __init__(self, i2c_bus, frequency):
        self._pca = PCA9685(i2c_bus)
        self._pca.frequency = frequency

        for channel in self._pca.channels:
            channel.duty_cycle = 0

        self._channels = [None] * 16

    def initialise_led(self, channel, start_brightness=0):
        """Initiate led brightness control class connected to specific channel.

        Args:
            channel: PCA9685 PWM channel that the LED receives its signal from.
            start_brightness: Initial brightness of the LED.
        """
        assert self._channels[channel] is None

        self._channels[channel] = LED(self._pca.channels[channel])
        self._channels[channel].set_brightness(start_brightness)

        return self._channels[channel]

    def initialise_servo(self, channel, lower=0.05, upper=0.1):
        """Initiate servo motor control class connected to specific channel.

        Args:
            channel: PCA9685 PWM channel that the servo receives its signal from.
            lower: Lower bound on PWM dutycyle that ideally achieves the point at 0 degrees of rotation. Default is 1ms or 5% of 50Hz.
            upper: upper bound on PWM dutycyle that ideally achieves the point at 180 degrees of rotation. Default is 2ms or 10% of 50Hz.
        """
        assert self._channels[channel] is None

        self._channels[channel] = Servo(
            self._pca.channels[channel], lower, upper
        )
        self._channels[channel].duty_cycle = 0

        return self._channels[channel]

    def change_frequency(self, frequency):
        """Change frequency of PWM signal.

        Args:
            frequency: The desired frequency that the PCA9685 PWM signal operates at between 50Hz and 1.6kHz.
        """
        self._pca.frequency = frequency


class LED(object):
    """LED class handles setting varying brightness levels of LEDs.

    Attributes:
        channel: PCA9685 PWM channel that the LED receives its signal from.
    """

    def __init__(self, channel):
        self._channel = channel

    def set_brightness(self, percentage):
        """Set brightness of LED signal to percentage between 0 and 100.

        Args:
            percentage: integer between 0 and 100 representing the desired brightness level.
        """
        assert percentage >= 0 and percentage <= 100
        self._channel.duty_cycle = MAX_DUTY_CYCLE * percentage // 100

    def set_off(self):
        """Set brightness of LED signal to minimum which is off."""
        self._channel.duty_cycle = 0

    def set_max(self):
        """Set brightness of LED signal to maximum."""
        self._channel.duty_cycle = MAX_DUTY_CYCLE


class Servo(object):
    """Servo class handles setting varying angles of servos.

    Attributes:
        channel: PCA9685 PWM channel that the servo receives its signal from.
        lower: Lower bound on PWM dutycyle that ideally achieves the point at 0 degrees of rotation.
        upper: upper bound on PWM dutycyle that ideally achieves the point at 180 degrees of rotation.
    """

    def __init__(self, channel, lower, upper):
        self._channel = channel
        self._lower = lower * MAX_DUTY_CYCLE
        self._upper = upper * MAX_DUTY_CYCLE

    def set_angle(self, angle):
        """Set angle of servo motor.

        Args:
            angle: Desired angle of the servo motor.
        """
        assert angle >= 0 and angle <= 180
        self._channel.duty_cycle = int(
            self._lower + (self._upper - self._lower) * angle // 180
        )

    def set_off(self):
        """Disable servo by setting input signal to constant low."""
        self._channel.duty_cycle = 0
