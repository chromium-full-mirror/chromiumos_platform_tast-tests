# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This is a modified version of the original freetronics libraries designed for usage with Arduino boards
# https://github.com/freetronics/DMD and https://github.com/freetronics/DMD2.

import digitalio


class DotMatrixDisplay(object):
    """Dot matrix display class handles interacting with the panels.

    Attributes:
        displays_wide: Number of panels wide in daisy-chain configuration.
        displays_high: Number of panels high in daisy-chain configuration.
        pin_SCK: GPIO pin that the DMD's SCK pin is connected to.
        pin_MOSI: GPIO pin that the DMD's MOSI pin is connected to.
        pin_A: GPIO pin that the DMD's A pin is connected to.
        pin_B: GPIO pin that the DMD's B pin is connected to.
        pin_SCLK: GPIO pin that the DMD's SCLK pin is connected to.
        pin_nOE: GPIO pin that the DMD's nOE pin is connected to.
        displays_total: Number of displays all together.
        row1: Offset of first row (0 indexed).
        row2: Offset of second row.
        row3: Offset of third row.
        screen: List used to store screen configuration.
        active_row: Row that is currently enabled.
    """

    _DMD_PIXELS_ACROSS = 32
    _DMD_PIXELS_DOWN = 16
    _DMD_BITSPERPIXEL = 1
    _DMD_RAM_SIZE_BYTES = (
        _DMD_PIXELS_ACROSS * _DMD_BITSPERPIXEL // 8
    ) * _DMD_PIXELS_DOWN

    def __init__(
        self,
        displays_wide,
        displays_high,
        pin_SCK,
        pin_MOSI,
        pin_A,
        pin_B,
        pin_SCLK,
        pin_nOE,
    ):

        self._displays_wide = displays_wide
        self._displays_high = displays_high
        self._displays_total = self._displays_wide * self._displays_high
        self._row1 = self._displays_total << 4
        self._row2 = 2 * self._row1
        self._row3 = 3 * self._row1
        self._screen = [0xFF] * self._displays_total * self._DMD_RAM_SIZE_BYTES

        self._pin_A = digitalio.DigitalInOut(pin_A)
        self._pin_B = digitalio.DigitalInOut(pin_B)
        self._pin_SCLK = digitalio.DigitalInOut(pin_SCLK)
        self._pin_nOE = digitalio.DigitalInOut(pin_nOE)
        self._pin_SCK = digitalio.DigitalInOut(pin_SCK)
        self._pin_MOSI = digitalio.DigitalInOut(pin_MOSI)

        self._pin_A.direction = digitalio.Direction.OUTPUT
        self._pin_B.direction = digitalio.Direction.OUTPUT
        self._pin_SCLK.direction = digitalio.Direction.OUTPUT
        self._pin_nOE.direction = digitalio.Direction.OUTPUT
        self._pin_SCK.direction = digitalio.Direction.OUTPUT
        self._pin_MOSI.direction = digitalio.Direction.OUTPUT

        self._pin_A.value = False
        self._pin_B.value = False
        self._pin_SCLK.value = False
        self._pin_nOE.value = False
        self._pin_SCK.value = False
        self._pin_MOSI.value = False

        self._active_row = 0

        self.set_screen(False)
        self.scan_display()

    def _latch_dmd_shift_reg_to_output(self):
        self._pin_SCLK.value = True
        self._pin_SCLK.value = False

    def _convert_coordinates(self, x, y):
        panel = (x // self._DMD_PIXELS_ACROSS) + (
            self._displays_wide * (y // self._DMD_PIXELS_DOWN)
        )
        x = (x % self._DMD_PIXELS_ACROSS) + (panel * self._DMD_PIXELS_ACROSS)
        y = y % self._DMD_PIXELS_DOWN

        return x, y

    def _update_pixel(self, x, y, status):
        assert x < (self._DMD_PIXELS_ACROSS * self._displays_wide) and y < (
            self._DMD_PIXELS_DOWN * self._displays_high
        )

        x, y = self._convert_coordinates(x, y)

        # Set pointer to DMD RAM byte to be modified.
        pointer = x // 8 + y * (self._displays_total << 2)

        lookup = 0x80 >> (x & 0x07)

        if status:
            self._screen[pointer] &= ~lookup  #  zero bit is pixel on
        else:
            self._screen[pointer] |= lookup  #  one bit is pixel off

    def _software_spi_write(self, byte):
        for _ in range(8):
            if byte & 0b10000000:
                self._pin_MOSI.value = True
            else:
                self._pin_MOSI.value = False
            self._pin_SCK.value = True
            byte <<= 1
            self._pin_SCK.value = False

    def scan_display(self):
        """Sends all the individual LED on/off configurations to the display."""
        # SPI transfer pixels to the display hardware shift registers.
        rowsize = self._displays_total << 2
        offset = rowsize * self._active_row

        for i in range(rowsize):
            self._software_spi_write(self._screen[offset + i + self._row3])
            self._software_spi_write(self._screen[offset + i + self._row2])
            self._software_spi_write(self._screen[offset + i + self._row1])
            self._software_spi_write(self._screen[offset + i])

        self._latch_dmd_shift_reg_to_output()

    def swap_active_rows(self):
        """Cycle's to the next row of the 4 possible active rows on the dot matrix display."""
        self._pin_A.value = self._active_row & 1
        self._pin_B.value = self._active_row & 2
        self._active_row = (self._active_row + 1) % 4

    def set_screen(self, status):
        """Sets all LED's on/off based on the provided status.

        Args:
            status: Boolean representing if LED should be on (True) or off (False).
        """
        if status:
            status = 0x00
        else:
            status = 0xFF

        for i, _ in enumerate(self._screen):
            self._screen[i] = status

    def set_panel(self, panel, status):
        """Sets all LED's in a given panel on/off based on the provided status.

        Args:
            panel: Integer representing which panel should be updated.
            status: Boolean representing if LED should be on (True) or off (False).
        """
        assert panel < self._displays_total and panel >= 0
        for x in range(self._DMD_PIXELS_ACROSS):
            for y in range(
                panel * self._DMD_PIXELS_DOWN, (panel + 1) * self._DMD_PIXELS_DOWN
            ):
                self._update_pixel(x, y, status)
