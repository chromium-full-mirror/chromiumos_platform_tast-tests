# Copyright 2023 The ChromiumOS Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This applications brings up a maximized window and fills it with blue.

import sys
from PyQt5.QtWidgets import QApplication, QWidget
from PyQt5.QtGui import QColor

app = QApplication(sys.argv)
w = QWidget()

# TODO(crbug.com/994009): Prefer to use maximize, which doesn't work currently.
# This workaround will break if tast tests start running on multi-monitor.
s = app.primaryScreen().size()
w.setGeometry(0, 0, s.width(), s.height())

p = w.palette()
p.setColor(w.backgroundRole(), QColor(0x12, 0x78, 0xEF))
w.setPalette(p)

w.show()
sys.exit(app.exec_())
