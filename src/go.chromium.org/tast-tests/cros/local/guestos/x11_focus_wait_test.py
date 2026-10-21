# Copyright 2026 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Unit tests for x11_focus_wait.py using a ctypes CFUNCTYPE fake libX11."""

import ctypes
import importlib.util
import pathlib
import sys
import unittest
from unittest import mock


_SCRIPT_PATH = pathlib.Path(__file__).resolve().with_name("x11_focus_wait.py")
_TITLE = "gtk3_copy_demo"


def _load_x11_focus_wait():
    """Loads x11_focus_wait.py as a module from the same directory."""
    spec = importlib.util.spec_from_file_location(
        "x11_focus_wait", _SCRIPT_PATH
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class _CFunc:
    """Wraps a Python callable in a ctypes.CFUNCTYPE matching its argtypes."""

    def __init__(self, fn):
        self.fn = fn
        self.argtypes = []
        self.restype = ctypes.c_int
        self._cfunc = None

    def __call__(self, *args):
        if self._cfunc is None:
            self._cfunc = ctypes.CFUNCTYPE(self.restype, *self.argtypes)(
                self.fn
            )
        return self._cfunc(*args)


class _FakeX11:
    """In-process ctypes fake of libX11.so.6 for testing x11_focus_wait.py."""

    def __init__(self, mode, title):
        self.mode = mode
        self.keep = []
        self.focus_win = 0
        self.get_focus_calls = 0
        self.set_focus_calls = 0
        self.closed = False
        # Window 101 is a mapped 1x1 degenerate helper window with the same
        # WM_NAME to verify _find_viewable_and_subtree skips 1x1 windows.
        # Window 102 is the 200x200 application window with child window 103.
        self.tree = {100: [101, 102], 101: [], 102: [103], 103: []}
        self.names = {
            101: ctypes.create_string_buffer(title.encode("utf-8")),
            102: ctypes.create_string_buffer(title.encode("utf-8")),
        }
        self.attrs = {
            101: (1, 1, 2),
            102: (200, 200, 0 if mode == "timeout_unmapped" else 2),
            103: (50, 50, 2),
        }
        self.XSetErrorHandler = _CFunc(self._set_error_handler)
        self.XOpenDisplay = _CFunc(self._open_display)
        self.XCloseDisplay = _CFunc(self._close_display)
        self.XDefaultRootWindow = _CFunc(self._default_root_window)
        self.XGetWindowAttributes = _CFunc(self._get_window_attributes)
        self.XQueryTree = _CFunc(self._query_tree)
        self.XFetchName = _CFunc(self._fetch_name)
        self.XFree = _CFunc(self._free)
        self.XGetInputFocus = _CFunc(self._get_input_focus)
        self.XSetInputFocus = _CFunc(self._set_input_focus)
        self.XSync = _CFunc(self._sync)

    def _set_error_handler(self, handler):
        self.keep.append(handler)
        return 0

    def _open_display(self, _name):
        return 1

    def _close_display(self, _dpy):
        self.closed = True
        return 0

    def _default_root_window(self, _dpy):
        return 100

    def _get_window_attributes(self, _dpy, win, attrs_ptr):
        if win not in self.attrs:
            return 0
        width, height, map_state = self.attrs[win]
        attrs_ptr[0].width = width
        attrs_ptr[0].height = height
        attrs_ptr[0].map_state = map_state
        return 1

    def _query_tree(
        self, _dpy, win, root_ret, parent_ret, children_ptr, num_children
    ):
        kids = self.tree.get(win, [])
        root_ret[0] = 100
        parent_ret[0] = 100
        num_children[0] = len(kids)
        if kids:
            arr = (ctypes.c_ulong * len(kids))(*kids)
            self.keep.append(arr)
            children_ptr[0] = ctypes.cast(arr, ctypes.POINTER(ctypes.c_ulong))
        else:
            children_ptr[0] = ctypes.POINTER(ctypes.c_ulong)()
        return 1

    def _fetch_name(self, _dpy, win, name_ptr):
        buf = self.names.get(win)
        if buf is None:
            name_ptr[0] = None
            return 0
        name_ptr[0] = ctypes.cast(buf, ctypes.c_void_p).value
        return 1

    def _free(self, _ptr):
        return 0

    def _get_input_focus(self, _dpy, focus_ret, revert_ret):
        self.get_focus_calls += 1
        if self.mode == "async_wm_focus" and self.get_focus_calls >= 3:
            self.focus_win = 103
        focus_ret[0] = self.focus_win
        revert_ret[0] = 2
        return 1

    def _set_input_focus(self, _dpy, win, _revert_to, _time_val):
        self.set_focus_calls += 1
        self.focus_win = win
        return 1

    def _sync(self, _dpy, _discard):
        return 1


class X11FocusWaitTest(unittest.TestCase):
    """Tests x11_focus_wait.main() across focus and timeout scenarios."""

    def _run_wait(self, mode, timeout_arg):
        module = _load_x11_focus_wait()
        fake_x11 = _FakeX11(mode, _TITLE)
        argv = ["x11_focus_wait.py", _TITLE, timeout_arg]
        with mock.patch.object(
            ctypes.cdll, "LoadLibrary", return_value=fake_x11
        ):
            with mock.patch.object(sys, "argv", argv):
                module.main()
        return fake_x11

    def test_async_wm_focus(self):
        """Returns once the window manager focuses a child window."""
        fake_x11 = self._run_wait("async_wm_focus", "3.0")
        self.assertTrue(fake_x11.closed)
        self.assertEqual(fake_x11.focus_win, 103)
        self.assertEqual(fake_x11.set_focus_calls, 0)

    def test_fallback_set_focus(self):
        """Requests XSetInputFocus on the viewable window after 0.5s."""
        fake_x11 = self._run_wait("fallback_set_focus", "3.0")
        self.assertTrue(fake_x11.closed)
        self.assertEqual(fake_x11.focus_win, 102)
        self.assertEqual(fake_x11.set_focus_calls, 1)

    def test_timeout_unmapped(self):
        """Exits with a diagnostic error when the target stays unmapped."""
        module = _load_x11_focus_wait()
        fake_x11 = _FakeX11("timeout_unmapped", _TITLE)
        argv = ["x11_focus_wait.py", _TITLE, "0.15"]
        with mock.patch.object(
            ctypes.cdll, "LoadLibrary", return_value=fake_x11
        ):
            with mock.patch.object(sys, "argv", argv):
                with self.assertRaises(SystemExit) as ctx:
                    module.main()
        self.assertTrue(fake_x11.closed)
        self.assertIn("timed out waiting for X11 focus", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
