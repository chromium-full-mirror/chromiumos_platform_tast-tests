# Copyright 2026 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Unit and headless Xvfb/Wayland tests for guestos copy/paste applets."""

import contextlib
import ctypes
import importlib.util
import io
import os
import pathlib
import shutil
import subprocess
import sys
import threading
import time
import types
import unittest
from unittest import mock


DATA_DIR = pathlib.Path(__file__).resolve().parent / "data"
# Use the system interpreter rather than sys.executable because virtualenvs /
# vpython3 environments do not carry Debian's python3-gi typelib bindings.
SYSTEM_PYTHON = "/usr/bin/python3"
IS_VIEWABLE = 2
XK_SPACE = 0x0020
_XERROR_HANDLER_TYPE = ctypes.CFUNCTYPE(
    ctypes.c_int, ctypes.c_void_p, ctypes.c_void_p
)


class _XWindowAttributes(ctypes.Structure):
    """Xlib XWindowAttributes layout matching <X11/Xlib.h>."""

    _fields_ = [
        ("x", ctypes.c_int),
        ("y", ctypes.c_int),
        ("width", ctypes.c_int),
        ("height", ctypes.c_int),
        ("border_width", ctypes.c_int),
        ("depth", ctypes.c_int),
        ("visual", ctypes.c_void_p),
        ("root", ctypes.c_ulong),
        ("c_class", ctypes.c_int),
        ("bit_gravity", ctypes.c_int),
        ("win_gravity", ctypes.c_int),
        ("backing_store", ctypes.c_int),
        ("backing_planes", ctypes.c_ulong),
        ("backing_pixel", ctypes.c_ulong),
        ("save_under", ctypes.c_int),
        ("colormap", ctypes.c_ulong),
        ("map_installed", ctypes.c_int),
        ("map_state", ctypes.c_int),
        ("all_event_masks", ctypes.c_long),
        ("your_event_mask", ctypes.c_long),
        ("do_not_propagate_mask", ctypes.c_long),
        ("override_redirect", ctypes.c_int),
        ("screen", ctypes.c_void_p),
    ]


def _expect(condition, message):
    if not condition:
        raise AssertionError(message)


def _has_system_gtk3() -> bool:
    if not os.path.exists(SYSTEM_PYTHON):
        return False
    env = {
        k: v
        for k, v in os.environ.items()
        if k not in ("DISPLAY", "WAYLAND_DISPLAY")
    }
    proc = subprocess.run(
        [
            SYSTEM_PYTHON,
            "-B",
            "-c",
            "import gi; gi.require_version('Gdk', '3.0'); "
            "gi.require_version('Gtk', '3.0'); "
            "from gi.repository import Gdk, Gtk",
        ],
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        timeout=5,
        check=False,
    )
    return proc.returncode == 0


def _load_applet(name, filename, clipboard, pending_cb=None):
    fake_gi = types.ModuleType("gi")
    required_versions = []
    fake_gi.require_version = lambda ns, ver: required_versions.append(
        (ns, ver)
    )
    fake_repo = types.ModuleType("gi.repository")

    class FakeWindow:
        """Minimal Gtk.Window stub for unit testing applet handlers."""

        def __init__(self, title=""):
            self.title = title
            self.closed = 0
            self.mapped = True
            self.size = (0, 0)

        def set_default_size(self, width, height):
            self.size = (width, height)

        def connect(self, _sig, _handler):
            pass

        def add(self, _child):
            pass

        def show_all(self):
            pass

        def present(self):
            pass

        def get_mapped(self):
            return self.mapped

        def close(self):
            self.closed += 1
            self.mapped = False

    fake_gtk = types.SimpleNamespace(
        Window=FakeWindow,
        Clipboard=types.SimpleNamespace(get=lambda _sel: clipboard),
        Label=lambda label="": label,
        events_pending=pending_cb or (lambda: False),
        main_iteration_do=mock.MagicMock(),
        main=lambda: None,
        main_quit=lambda *_args: None,
    )
    fake_gdk = types.SimpleNamespace(
        SELECTION_CLIPBOARD="CLIPBOARD",
        EVENT_STOP=True,
    )
    fake_repo.Gtk = fake_gtk
    fake_repo.Gdk = fake_gdk

    with mock.patch.dict(
        sys.modules, {"gi": fake_gi, "gi.repository": fake_repo}
    ):
        spec = importlib.util.spec_from_file_location(name, DATA_DIR / filename)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return mod, fake_gtk, fake_gdk, required_versions


def _watch_stderr(proc, needle):
    lines = []
    found = threading.Event()

    def reader():
        for line in proc.stderr:
            lines.append(line)
            if needle in line:
                found.set()

    thread = threading.Thread(target=reader, daemon=True)
    thread.start()
    return lines, found, thread


def _window_name(x11, dpy, win):
    name_ptr = ctypes.c_void_p()
    if (
        not x11.XFetchName(dpy, win, ctypes.byref(name_ptr))
        or not name_ptr.value
    ):
        return None
    try:
        return ctypes.string_at(name_ptr.value).decode("utf-8", "replace")
    finally:
        x11.XFree(name_ptr)


def _is_viewable(x11, dpy, win):
    attrs = _XWindowAttributes()
    return (
        bool(x11.XGetWindowAttributes(dpy, win, ctypes.byref(attrs)))
        and attrs.map_state == IS_VIEWABLE
    )


def _child_windows(x11, dpy, win):
    root_ret = ctypes.c_ulong(0)
    parent_ret = ctypes.c_ulong(0)
    children = ctypes.POINTER(ctypes.c_ulong)()
    nchildren = ctypes.c_uint(0)
    ok = x11.XQueryTree(
        dpy,
        win,
        ctypes.byref(root_ret),
        ctypes.byref(parent_ret),
        ctypes.byref(children),
        ctypes.byref(nchildren),
    )
    if not ok or not nchildren.value:
        return []
    try:
        return [children[idx] for idx in range(nchildren.value)]
    finally:
        x11.XFree(children)


def _find_viewable_window_once(x11, dpy, root, name):
    stack = [root]
    while stack:
        win = stack.pop()
        # Wait for IsViewable so XSetInputFocus does not raise BadMatch
        # before XMapWindow finishes.
        if _window_name(x11, dpy, win) == name and _is_viewable(x11, dpy, win):
            return win
        stack.extend(_child_windows(x11, dpy, win))
    return None


def _run_xvfb_e2e_worker():
    x11 = ctypes.cdll.LoadLibrary("libX11.so.6")
    xtst = ctypes.cdll.LoadLibrary("libXtst.so.6")
    x11.XSetErrorHandler.argtypes = [_XERROR_HANDLER_TYPE]
    x11.XSetErrorHandler.restype = ctypes.c_void_p
    x11.XOpenDisplay.argtypes = [ctypes.c_char_p]
    x11.XOpenDisplay.restype = ctypes.c_void_p
    x11.XCloseDisplay.argtypes = [ctypes.c_void_p]
    x11.XDefaultRootWindow.argtypes = [ctypes.c_void_p]
    x11.XDefaultRootWindow.restype = ctypes.c_ulong
    x11.XGetWindowAttributes.argtypes = [
        ctypes.c_void_p,
        ctypes.c_ulong,
        ctypes.POINTER(_XWindowAttributes),
    ]
    x11.XQueryTree.argtypes = [
        ctypes.c_void_p,
        ctypes.c_ulong,
        ctypes.POINTER(ctypes.c_ulong),
        ctypes.POINTER(ctypes.c_ulong),
        ctypes.POINTER(ctypes.POINTER(ctypes.c_ulong)),
        ctypes.POINTER(ctypes.c_uint),
    ]
    x11.XFetchName.argtypes = [
        ctypes.c_void_p,
        ctypes.c_ulong,
        ctypes.POINTER(ctypes.c_void_p),
    ]
    x11.XFree.argtypes = [ctypes.c_void_p]
    x11.XSetInputFocus.argtypes = [
        ctypes.c_void_p,
        ctypes.c_ulong,
        ctypes.c_int,
        ctypes.c_ulong,
    ]
    x11.XKeysymToKeycode.argtypes = [ctypes.c_void_p, ctypes.c_ulong]
    x11.XKeysymToKeycode.restype = ctypes.c_uint
    x11.XFlush.argtypes = [ctypes.c_void_p]
    x11.XSync.argtypes = [ctypes.c_void_p, ctypes.c_int]
    xtst.XTestFakeKeyEvent.argtypes = [
        ctypes.c_void_p,
        ctypes.c_uint,
        ctypes.c_int,
        ctypes.c_ulong,
    ]

    # Ignore transient BadWindow errors while walking short-lived helper windows
    # in XQueryTree / XFetchName. Keep a local reference alive across X11 calls.
    err_handler = _XERROR_HANDLER_TYPE(lambda _dpy, _err: 0)
    x11.XSetErrorHandler(err_handler)

    dpy = x11.XOpenDisplay(None)
    if not dpy:
        raise RuntimeError("Failed to open X11 display")
    try:
        root = x11.XDefaultRootWindow(dpy)
        space_kc = x11.XKeysymToKeycode(dpy, XK_SPACE)

        def find_win(name):
            deadline = time.monotonic() + 5.0
            while time.monotonic() < deadline:
                win = _find_viewable_window_once(x11, dpy, root, name)
                if win is not None:
                    return win
                time.sleep(0.03)
            raise AssertionError(f"Window not found: {name}")

        def burst_space(win):
            x11.XSetInputFocus(dpy, win, 1, 0)
            x11.XSync(dpy, 0)
            for _ in range(3):
                xtst.XTestFakeKeyEvent(dpy, space_kc, 1, 0)
                xtst.XTestFakeKeyEvent(dpy, space_kc, 0, 0)
                x11.XFlush(dpy)
                time.sleep(0.01)

        env = dict(os.environ, GDK_BACKEND="x11", NO_AT_BRIDGE="1")
        with contextlib.ExitStack() as stack:
            # Launch and map both windows before sending keys so copy startup
            # time does not eat into paste_applet's 5s retry budget; CLIPBOARD
            # stays unowned until copy_win receives a keypress.
            paste_proc = stack.enter_context(
                subprocess.Popen(
                    [SYSTEM_PYTHON, "-B", str(DATA_DIR / "paste_applet.py")],
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                    env=env,
                )
            )
            # ExitStack unwinds LIFO, so kill() runs before Popen.__exit__()
            # waits for the child process on an unexpected exception.
            stack.callback(paste_proc.kill)
            paste_lines, paste_latched, paste_thread = _watch_stderr(
                paste_proc, "ignoring repeated key press"
            )

            copy_proc = stack.enter_context(
                subprocess.Popen(
                    [
                        SYSTEM_PYTHON,
                        "-B",
                        str(DATA_DIR / "copy_applet.py"),
                        "delayed x11 text",
                    ],
                    stderr=subprocess.PIPE,
                    text=True,
                    env=env,
                )
            )
            stack.callback(copy_proc.kill)
            copy_lines, _, copy_thread = _watch_stderr(
                copy_proc, "ignoring repeated key press"
            )

            paste_win = find_win("gtk3_paste_demo")
            copy_win = find_win("gtk3_copy_demo")

            burst_space(paste_win)
            if not paste_latched.wait(5.0):
                paste_proc.terminate()
                paste_thread.join(timeout=2.0)
                raise AssertionError(
                    "paste never logged a repeated press: "
                    + "".join(paste_lines)
                )

            burst_space(copy_win)
            paste_proc.wait(timeout=5)
            paste_thread.join(timeout=2.0)
            out = paste_proc.stdout.read()
            err = "".join(paste_lines)
            _expect(paste_proc.returncode == 0, err)
            _expect(out == "delayed x11 text", out)
            _expect("ignoring repeated key press" in err, err)

            copy_proc.terminate()
            copy_proc.wait(timeout=2)
            copy_thread.join(timeout=2.0)
            copy_err = "".join(copy_lines)
            _expect("ignoring repeated key press" in copy_err, copy_err)
    finally:
        x11.XCloseDisplay(dpy)


class CopyPasteAppletUnitTest(unittest.TestCase):
    """Unit tests covering key-repeat guards, retry bounds, and unmapping."""

    def test_copy_ignores_repeated_key_press(self):
        clipboard = mock.MagicMock()
        mod, _, gdk, req = _load_applet(
            "copy_applet_u1", "copy_applet.py", clipboard
        )
        self.assertIn(("Gdk", "3.0"), req)
        self.assertIn(("Gtk", "3.0"), req)
        mod.main(["prog", "ignored_main_arg"])
        win = mod.CopyWindow("abc")
        err = io.StringIO()
        with mock.patch("sys.stderr", err):
            self.assertEqual(win.copy("w", "e1"), gdk.EVENT_STOP)
            self.assertEqual(win.copy("w", "e2"), gdk.EVENT_STOP)
        self.assertEqual(
            clipboard.set_text.call_args_list, [mock.call("abc", -1)]
        )
        # Sommelier's CLIPBOARD_MANAGER handler is a stub that does not persist
        # selections, and gtk_clipboard_store() runs a nested main loop that can
        # block up to 10s past delete-event.
        self.assertEqual(clipboard.store.call_count, 0)
        self.assertIn("ignoring repeated key press", err.getvalue())

    def test_paste_retries_beyond_one_second_and_blocks_reentrancy(self):
        clock = [10.0]
        clipboard = mock.MagicMock()
        pumped = [True, False]
        mod, gtk, gdk, _ = _load_applet(
            "paste_applet_u1",
            "paste_applet.py",
            clipboard,
            pending_cb=lambda: pumped.pop(0) if pumped else False,
        )
        mod.main()
        win = mod.PasteWindow()
        min_wait_sec = 1.5
        retries = int(min_wait_sec / mod.PASTE_RETRY_INTERVAL_SEC) + 1
        responses = [None] * retries + ["copied text"]
        calls = []

        def wait_side_effect():
            idx = len(calls)
            calls.append(idx)
            if idx == 0:
                win.paste("w", "repeat_press")
            return responses[idx]

        clipboard.wait_for_text.side_effect = wait_side_effect
        out = io.StringIO()
        err = io.StringIO()
        with contextlib.ExitStack() as stack:
            stack.enter_context(
                mock.patch.object(
                    time, "monotonic", side_effect=lambda: clock[0]
                )
            )
            stack.enter_context(
                mock.patch.object(
                    time,
                    "sleep",
                    side_effect=lambda dt: clock.__setitem__(0, clock[0] + dt),
                )
            )
            stack.enter_context(mock.patch("sys.stdout", out))
            stack.enter_context(mock.patch("sys.stderr", err))
            self.assertEqual(win.paste("w", "first_press"), gdk.EVENT_STOP)

        self.assertEqual(len(calls), retries + 1)
        self.assertGreater(clock[0] - 10.0, min_wait_sec)
        self.assertTrue(gtk.main_iteration_do.called)
        self.assertEqual(out.getvalue(), "copied text")
        self.assertIn("ignoring repeated key press", err.getvalue())
        self.assertEqual(win.closed, 1)

    def test_paste_exits_after_single_30s_selection_timeout(self):
        clock = [50.0]
        clipboard = mock.MagicMock()

        def stalled_wait():
            clock[0] += 30.0

        clipboard.wait_for_text.side_effect = stalled_wait
        mod, _, gdk, _ = _load_applet(
            "paste_applet_u2", "paste_applet.py", clipboard
        )
        win = mod.PasteWindow()
        out = io.StringIO()
        err = io.StringIO()
        with contextlib.ExitStack() as stack:
            stack.enter_context(
                mock.patch.object(
                    time, "monotonic", side_effect=lambda: clock[0]
                )
            )
            stack.enter_context(mock.patch("sys.stdout", out))
            stack.enter_context(mock.patch("sys.stderr", err))
            self.assertEqual(win.paste("w", "first_press"), gdk.EVENT_STOP)

        self.assertEqual(clipboard.wait_for_text.call_count, 1)
        self.assertEqual(out.getvalue(), "None")
        self.assertIn("clipboard has no text", err.getvalue())
        self.assertEqual(win.closed, 1)

    def test_paste_stops_cleanly_if_unmapped_during_pump(self):
        clipboard = mock.MagicMock(
            wait_for_text=mock.MagicMock(return_value=None)
        )
        mod, gtk, _, _ = _load_applet(
            "paste_applet_u3",
            "paste_applet.py",
            clipboard,
            pending_cb=lambda: True,
        )
        win = mod.PasteWindow()
        gtk.main_iteration_do.side_effect = lambda _b: setattr(
            win, "mapped", False
        )
        out = io.StringIO()
        err = io.StringIO()
        with mock.patch("sys.stdout", out), mock.patch("sys.stderr", err):
            win.paste("w", "first_press")

        self.assertEqual(clipboard.wait_for_text.call_count, 1)
        self.assertEqual(out.getvalue(), "")
        self.assertIn("window unmapped before paste completed", err.getvalue())
        self.assertEqual(win.closed, 0)


class CopyPasteAppletGiTest(unittest.TestCase):
    """Real PyGObject tests covering Wayland imports and X11 Xvfb E2E."""

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        if not _has_system_gtk3():
            raise unittest.SkipTest("Requires system python3-gi + Gtk 3.0")

    def test_wayland_backend_import_and_class_defs(self):
        env = {
            k: v
            for k, v in os.environ.items()
            if k not in ("DISPLAY", "WAYLAND_DISPLAY")
        }
        env["GDK_BACKEND"] = "wayland"
        script = (
            "import sys\n"
            f"sys.path.insert(0, {str(DATA_DIR)!r})\n"
            "import copy_applet\n"
            "import paste_applet\n"
            "assert copy_applet.Gdk._version == '3.0'\n"
            "assert copy_applet.Gtk._version == '3.0'\n"
            "assert paste_applet.Gdk._version == '3.0'\n"
            "assert paste_applet.Gtk._version == '3.0'\n"
        )
        proc = subprocess.run(
            [SYSTEM_PYTHON, "-B", "-Werror", "-c", script],
            env=env,
            capture_output=True,
            text=True,
            timeout=10,
            check=False,
        )
        self.assertEqual(proc.returncode, 0, msg=proc.stderr or proc.stdout)

    @unittest.skipUnless(shutil.which("xvfb-run"), "Requires xvfb-run")
    def test_x11_copy_and_delayed_paste(self):
        test_path = pathlib.Path(__file__).resolve()
        proc = subprocess.run(
            [
                "xvfb-run",
                "-a",
                SYSTEM_PYTHON,
                "-B",
                str(test_path),
                "--xvfb-worker",
            ],
            capture_output=True,
            text=True,
            timeout=20,
            check=False,
        )
        self.assertEqual(
            proc.returncode, 0, msg=f"{test_path}: {proc.stderr or proc.stdout}"
        )


if __name__ == "__main__":
    if "--xvfb-worker" in sys.argv:
        _run_xvfb_e2e_worker()
    else:
        unittest.main()
