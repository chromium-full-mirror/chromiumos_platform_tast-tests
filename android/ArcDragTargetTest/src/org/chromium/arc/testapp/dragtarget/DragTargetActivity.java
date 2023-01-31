/*
 * Copyright 2020 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.dragtarget;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipDescription;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.os.PersistableBundle;
import android.view.DragEvent;
import android.widget.TextView;

public class DragTargetActivity extends Activity {
    private TextView mDroppedDataView;
    private TextView mDroppedArea;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_dragtarget);

        mDroppedDataView = (TextView) findViewById(R.id.dropped_data_view);
        mDroppedArea = (TextView) findViewById(R.id.dropped_area);

        mDroppedArea.setOnDragListener(
                (view, event) -> {
                    switch (event.getAction()) {
                        case DragEvent.ACTION_DRAG_STARTED:
                            return true;
                        case DragEvent.ACTION_DROP:
                            // Do not use ClipData.toString() because it does not contain clip data
                            // contents information for privacy reasons.
                            mDroppedDataView.setText(clipDataToString(event.getClipData()));
                            return true;
                    }
                    return false;
                });
    }

    private static String clipDataToString(ClipData clipData) {
        StringBuilder b = new StringBuilder(128);

        b.append("ClipData { ");
        boolean first;
        ClipDescription description = clipData.getDescription();
        if (description != null) {
            first = !clipDescriptionToString(b, description);
        } else {
            first = true;
        }
        for (int i = 0; i < clipData.getItemCount(); i++) {
            if (!first) {
                b.append(' ');
            }
            first = false;
            b.append('{');
            clipDataItemToString(b, clipData.getItemAt(i));
            b.append('}');
        }
        b.append(" }");

        return b.toString();
    }

    private static boolean clipDescriptionToString(StringBuilder b, ClipDescription description) {
        boolean first = true;
        final int size = description.getMimeTypeCount();
        for (int i = 0; i < size; i++) {
            if (!first) {
                b.append(' ');
            }
            first = false;
            b.append(description.getMimeType(i));
        }
        CharSequence label = description.getLabel();
        if (label != null) {
            if (!first) {
                b.append(' ');
            }
            first = false;
            b.append('"');
            b.append(label);
            b.append('"');
        }
        PersistableBundle extras = description.getExtras();
        if (extras != null) {
            if (!first) {
                b.append(' ');
            }
            first = false;
            b.append(extras.toString());
        }
        return !first;
    }

    private static void clipDataItemToString(StringBuilder b, ClipData.Item item) {
        String htmlText = item.getHtmlText();
        if (htmlText != null) {
            b.append("H:");
            b.append(htmlText);
        }
        CharSequence text = item.getText();
        if (text != null) {
            b.append("T:");
            b.append(text);
        }
        Uri uri = item.getUri();
        if (uri != null) {
            b.append("U:");
            b.append(uri);
        }
        Intent intent = item.getIntent();
        if (intent != null) {
            b.append("I:");
            b.append(intent.toString());
        }
    }
}
