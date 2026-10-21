# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Opens a window and pastes text from the clipboard on keypress."""

# pylint: disable=import-error,wrong-import-position

import sys
import time

import gi


# Gdk is imported before Gtk (sorted imports), so pin it explicitly or
# PyGObject may pick Gdk 4.0 and break the Gtk 3.0 import below.
gi.require_version("Gdk", "3.0")
gi.require_version("Gtk", "3.0")
from gi.repository import Gdk
from gi.repository import Gtk


# Upper bound on retrying an empty clipboard right after map.
PASTE_RETRY_TIMEOUT_SEC = 5.0
# Pause between retries while pending GTK events are pumped.
PASTE_RETRY_INTERVAL_SEC = 0.2
# Sleep granularity inside the GTK event-pumping loop.
PASTE_PUMP_SLEEP_SEC = 0.02


class PasteWindow(Gtk.Window):
    """Window that prints clipboard text and closes on key press."""

    def __init__(self):
        super().__init__(title="gtk3_paste_demo")
        self.set_default_size(200, 200)

        self.clipboard = Gtk.Clipboard.get(Gdk.SELECTION_CLIPBOARD)
        self.pasted = False
        self.connect("key_press_event", self.paste)

        label = Gtk.Label(label="Paste Applet")
        self.add(label)

    def paste(self, *_args):
        # wait_for_text() runs a nested main loop, so a repeated
        # key_press_event could re-enter paste() and print/close twice.
        if self.pasted:
            print("ignoring repeated key press", file=sys.stderr, flush=True)
            return Gdk.EVENT_STOP
        self.pasted = True
        # Right after map, Sommelier may not have claimed X11 CLIPBOARD for
        # the host selection yet, so wait_for_text() can transiently return
        # None. Retry until the deadline; a call that blocks until GTK's
        # ~30s selection timeout is not retried.
        deadline = time.monotonic() + PASTE_RETRY_TIMEOUT_SEC
        text = self.clipboard.wait_for_text()
        while text is None and time.monotonic() < deadline:
            pump_until = min(
                time.monotonic() + PASTE_RETRY_INTERVAL_SEC, deadline
            )
            while time.monotonic() < pump_until:
                # Check get_mapped() in the pump condition too so continuous
                # pending GDK events cannot spin past an unmap before breaking.
                while Gtk.events_pending() and self.get_mapped():
                    Gtk.main_iteration_do(False)
                # get_mapped() turns False only on app hide/destroy (not WM
                # iconification); abort early if delete-event tore us down.
                if not self.get_mapped():
                    print(
                        "window unmapped before paste completed",
                        file=sys.stderr,
                        flush=True,
                    )
                    return Gdk.EVENT_STOP
                time.sleep(PASTE_PUMP_SLEEP_SEC)
            text = self.clipboard.wait_for_text()
        if text is None:
            print("clipboard has no text", file=sys.stderr, flush=True)
        print(text, end="", flush=True)
        self.close()
        return Gdk.EVENT_STOP


def main():
    """Opens the paste demo window and runs the GTK main loop."""
    window = PasteWindow()
    window.show_all()
    window.present()
    window.connect("delete-event", Gtk.main_quit)
    Gtk.main()


if __name__ == "__main__":
    main()
