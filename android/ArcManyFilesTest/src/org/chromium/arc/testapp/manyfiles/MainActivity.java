/*
 * Copyright 2023 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.manyfiles;

import android.app.Activity;
import android.os.Bundle;
import android.util.Log;
import android.widget.TextView;

import java.io.File;

/**
 * Main activity for the ArcManyFilesTest app.
 *
 * <p>Used by tast test to read files and create another file in a directory containing a large
 * number of files.
 */
public class MainActivity extends Activity {
    private static final String TAG = "ArcManyFilesTest";

    // The message to be displayed when the app-side checks succeed.
    // This should be kept in sync with many_files.go.
    private static final String MESSAGE_SUCCESS = "PASS";

    // This should be kept in sync with many_files.go.
    private static final String FILENAME_CREATED_BY_APP = "storage.txt";

    // This should be kept in sync with many_files.go.
    private static final int NUMBER_OF_FILES = 10000;

    private TextView mResult;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        setContentView(R.layout.main_activity);
        mResult = findViewById(R.id.result);

        final Bundle extras = getIntent().getExtras();
        if (extras == null) {
            Log.e(TAG, "No extra data in intent");
            mResult.setText("No extra data in intent");
            return;
        }
        final String directoryPath = extras.getString("path");

        Thread thread =
                new Thread(
                        () -> {
                            final String msg = testOnWorkerThread(directoryPath);
                            if (!MESSAGE_SUCCESS.equals(msg)) {
                                Log.e(TAG, msg);
                            }
                            runOnUiThread(() -> mResult.setText(msg));
                        });
        thread.start();
    }

    private String testOnWorkerThread(final String directoryPath) {
        Log.i(TAG, "Checking files under " + directoryPath);
        for (int i = 0; i < NUMBER_OF_FILES; i++) {
            final String filename = String.format("storage_%d", i);
            File file = new File(directoryPath, filename);
            if (!file.exists()) {
                return String.format("Failed to open %s", filename);
            }
        }

        Log.i(TAG, "Creating another file under " + directoryPath);
        File file = new File(directoryPath, FILENAME_CREATED_BY_APP);
        try {
            if (!file.createNewFile()) {
                return String.format(
                        "Failed to create file %s as it already exists", FILENAME_CREATED_BY_APP);
            }
        } catch (Exception e) {
            return String.format("Failed to create file %s: %s", FILENAME_CREATED_BY_APP, e);
        }
        return MESSAGE_SUCCESS;
    }
}
