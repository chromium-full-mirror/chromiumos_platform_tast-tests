/*
 * Copyright 2023 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.arcstandardizedinputtest;

import android.content.Context;
import android.util.AttributeSet;
import android.util.Log;
import android.view.KeyEvent;
import android.widget.EditText;

public class KeyLoggingEditText extends EditText {
    private static final String TAG = "KeyLoggingEditText";

    public KeyLoggingEditText(Context context) {
        super(context);
    }

    public KeyLoggingEditText(Context context, AttributeSet attrs) {
        super(context, attrs);
    }

    public KeyLoggingEditText(Context context, AttributeSet attrs, int defStyleAttr) {
        super(context, attrs, defStyleAttr);
    }

    public KeyLoggingEditText(
            Context context, AttributeSet attrs, int defStyleAttr, int defStyleRes) {
        super(context, attrs, defStyleAttr, defStyleRes);
    }

    @Override
    public boolean dispatchKeyEventPreIme(KeyEvent event) {
        Log.w(TAG, "dispatchKeyEventPreIme: event=" + event);
        return super.dispatchKeyEventPreIme(event);
    }
}
