/*
 * Copyright 2023 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.appmanagement;

import android.app.Activity;

/**
 * Empty main activity. The app is never launched in the test, but we need a
 * main activity to appear as an app in Ash.
 */
public class MainActivity extends Activity {}
