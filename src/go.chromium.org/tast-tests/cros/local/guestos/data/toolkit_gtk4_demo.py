# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This applications brings up a maximized window and fills it with blue.

import gi

gi.require_version("Gtk", "4.0")
from gi.repository import Gtk, Gdk

STYLESHEET = """
window {
  background-color: #1278EF;
}
"""


class DemoWindow(Gtk.ApplicationWindow):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.set_title("gtk4_demo")


class Demo(Gtk.Application):
    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        self.connect("activate", self.on_activate)

    def on_activate(self, app):
        self.window = DemoWindow(application=app)
        self.window.present()
        self.window.maximize()


css_provider = Gtk.CssProvider()
css_provider.load_from_data(STYLESHEET.encode())
Gtk.StyleContext.add_provider_for_display(
    Gdk.Display.get_default(),
    css_provider,
    Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
)
app = Demo()
app.run()
