# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Opens a window and copies command-line data to the clipboard on keypress."""

# pylint: disable=import-error,wrong-import-position

import sys

import gi


# Gdk is imported before Gtk (sorted imports), so pin it explicitly or
# PyGObject may pick Gdk 4.0 and break the Gtk 3.0 import below.
gi.require_version("Gdk", "3.0")
gi.require_version("Gtk", "3.0")
from gi.repository import Gdk
from gi.repository import Gtk


class CopyWindow(Gtk.Window):
    """Window that copies command-line data to the clipboard on key press."""

    def __init__(self, data):
        super().__init__(title="gtk3_copy_demo")
        self.set_default_size(200, 200)

        self.clipboard = Gtk.Clipboard.get(Gdk.SELECTION_CLIPBOARD)
        self.data = data
        self.copied = False
        self.connect("key_press_event", self.copy)

        label = Gtk.Label(label="Copy Applet")
        self.add(label)

    def copy(self, *_args):
        # Ignore repeated key_press_event signals. Under compositor load the
        # key release can arrive late, and Xwayland's software auto-repeat
        # (driven by wl_keyboard.repeat_info) then synthesizes extra presses.
        # A second set_text() would make this window the X11 CLIPBOARD owner
        # again right before CloseAllWindows destroys it, after which
        # Sommelier sees no owner and clears the host Wayland selection.
        if self.copied:
            print("ignoring repeated key press", file=sys.stderr, flush=True)
            return Gdk.EVENT_STOP
        self.copied = True
        self.clipboard.set_text(self.data, -1)
        return Gdk.EVENT_STOP


def main(argv=None):
    """Opens the copy demo window and runs the GTK main loop."""
    args = sys.argv if argv is None else argv
    window = CopyWindow(args[1])
    window.show_all()
    window.present()
    window.connect("delete-event", Gtk.main_quit)
    Gtk.main()


if __name__ == "__main__":
    main()
