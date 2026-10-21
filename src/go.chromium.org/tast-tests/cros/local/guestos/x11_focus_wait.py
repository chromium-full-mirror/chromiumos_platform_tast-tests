# Copyright 2026 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Waits for a viewable X11 window with the given WM_NAME to hold X11 focus."""

import ctypes
import sys
import time


_IS_VIEWABLE = 2
_REVERT_TO_PARENT = 2
_CURRENT_TIME = 0
_FALLBACK_SET_FOCUS_DELAY_SEC = 0.5
_POLL_INTERVAL_SEC = 0.05

_XERROR_HANDLER_TYPE = ctypes.CFUNCTYPE(
    ctypes.c_int, ctypes.c_void_p, ctypes.c_void_p
)


class _XWindowAttributes(ctypes.Structure):
    """C layout of Xlib XWindowAttributes."""

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


def _configure_xlib(x11):
    """Sets ctypes signatures for the libX11 functions used during polling."""
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
    x11.XGetInputFocus.argtypes = [
        ctypes.c_void_p,
        ctypes.POINTER(ctypes.c_ulong),
        ctypes.POINTER(ctypes.c_int),
    ]
    x11.XSetInputFocus.argtypes = [
        ctypes.c_void_p,
        ctypes.c_ulong,
        ctypes.c_int,
        ctypes.c_ulong,
    ]
    x11.XSync.argtypes = [ctypes.c_void_p, ctypes.c_int]


def _query_children(x11, dpy, win):
    """Returns direct child window XIDs of win."""
    root_ret = ctypes.c_ulong(0)
    parent_ret = ctypes.c_ulong(0)
    children_ptr = ctypes.POINTER(ctypes.c_ulong)()
    num_children = ctypes.c_uint(0)
    ok = x11.XQueryTree(
        dpy,
        win,
        ctypes.byref(root_ret),
        ctypes.byref(parent_ret),
        ctypes.byref(children_ptr),
        ctypes.byref(num_children),
    )
    if not ok or not children_ptr:
        return []
    result = [children_ptr[i] for i in range(num_children.value)]
    x11.XFree(children_ptr)
    return result


def _find_viewable_and_subtree(x11, dpy, root, target_name):
    """Finds a viewable window named target_name and collects its subtree."""
    stack = [root]
    while stack:
        win = stack.pop()
        name_ptr = ctypes.c_void_p()
        if x11.XFetchName(dpy, win, ctypes.byref(name_ptr)) and name_ptr.value:
            try:
                wm_name = ctypes.string_at(name_ptr.value).decode(
                    "utf-8", "replace"
                )
            finally:
                x11.XFree(name_ptr)
            attrs = _XWindowAttributes()
            # Skip degenerate 1x1 helper/leader windows so we do not direct
            # X11 input focus away from the real application window.
            if (
                wm_name == target_name
                and x11.XGetWindowAttributes(dpy, win, ctypes.byref(attrs))
                and attrs.map_state == _IS_VIEWABLE
                and attrs.width > 1
                and attrs.height > 1
            ):
                subtree = {win}
                sub_stack = [win]
                while sub_stack:
                    cur = sub_stack.pop()
                    for child in _query_children(x11, dpy, cur):
                        subtree.add(child)
                        sub_stack.append(child)
                return win, subtree
        stack.extend(_query_children(x11, dpy, win))
    return 0, set()


def main():
    """Polls until target_name is viewable and holds X11 input focus."""
    target_name = sys.argv[1]
    timeout_sec = float(sys.argv[2])

    x11 = ctypes.cdll.LoadLibrary("libX11.so.6")
    _configure_xlib(x11)
    # Ignore transient BadWindow errors while walking short-lived helper windows
    # in XQueryTree / XFetchName. Keep err_handler alive across X11 calls.
    err_handler = _XERROR_HANDLER_TYPE(lambda _dpy, _err: 0)
    x11.XSetErrorHandler(err_handler)

    dpy = x11.XOpenDisplay(None) or x11.XOpenDisplay(b":0")
    if not dpy:
        sys.exit("failed to open X11 display")

    try:
        root = x11.XDefaultRootWindow(dpy)
        deadline = time.monotonic() + timeout_sec
        tracked_win = 0
        viewable_since = None
        last_focus = 0

        while time.monotonic() < deadline:
            win, subtree = _find_viewable_and_subtree(
                x11, dpy, root, target_name
            )
            if win:
                if win != tracked_win or viewable_since is None:
                    tracked_win = win
                    viewable_since = time.monotonic()
                focus_win = ctypes.c_ulong(0)
                revert_to = ctypes.c_int(0)
                x11.XGetInputFocus(
                    dpy, ctypes.byref(focus_win), ctypes.byref(revert_to)
                )
                last_focus = focus_win.value
                if last_focus in subtree:
                    x11.XSync(dpy, 0)
                    return
                # If Sommelier received xdg_toplevel.configure(ACTIVATED) before
                # MapRequest set window->managed, Xwayland focus stays XCB_NONE.
                # Reset viewable_since after requesting focus so retries stay
                # throttled to _FALLBACK_SET_FOCUS_DELAY_SEC rather than 50ms.
                if (
                    time.monotonic() - viewable_since
                    >= _FALLBACK_SET_FOCUS_DELAY_SEC
                ):
                    x11.XSetInputFocus(
                        dpy, win, _REVERT_TO_PARENT, _CURRENT_TIME
                    )
                    x11.XSync(dpy, 0)
                    viewable_since = time.monotonic()
            else:
                tracked_win = 0
                viewable_since = None
            time.sleep(_POLL_INTERVAL_SEC)
        sys.exit(
            f"timed out waiting for X11 focus on {target_name!r} "
            f"(win={tracked_win:#x}, focus={last_focus:#x})"
        )
    finally:
        x11.XCloseDisplay(dpy)


if __name__ == "__main__":
    main()
