/*
 * Copyright 2023 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.fileeditor;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;
import android.util.Log;
import android.widget.Button;
import android.widget.TextView;
import android.view.View;
import android.view.View.OnClickListener;

import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.Scanner;

/**
 * Main activity for the ArcFileEditorTest app.
 *
 * <p>Used by tast test to read and edit files shared with ChromeOS.
 * User can specify a file to read by including URI of the file in the intent to
 * launch this activity. This activity then reads the file and displays its
 * content in TextView for validation.
 * When a file's content is displayed on UI, pressing the "Modify file" button
 * modifies the file by appending string to it and updates the file content
 * displayed on the UI.
 */
public class MainActivity extends Activity {
    public static final String LOG_TAG = "ArcFileEditorTest";

    // The string to be appended at the end of the currently displayed file
    // when the "Modify file" button is pressed.
    // This should be kept in sync with storage.go.
    private static final String CONTENT_APPEND = ", this is added by Android";

    // Displays the intent action that launched this activity (for debugging).
    private TextView mActionView;
    // Displays the URI of the file (for debugging).
    private TextView mUriView;
    // Displays the content of the opened file.
    private TextView mFileContentView;

    // The URI of the currently displayed file.
    private Uri mUri;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        setContentView(R.layout.main_activity);
        mActionView = findViewById(R.id.action);
        mUriView = findViewById(R.id.uri);
        mFileContentView = findViewById(R.id.file_content);

        Intent intent = getIntent();

        String action = intent.getAction();
        mActionView.setText(action);
        Log.i(LOG_TAG, "Action = " + action);

        Uri uri = intent.getData();
        if (uri == null) {
            return;
        }
        mUri = uri;
        mUriView.setText(uri.toString());
        Log.i(LOG_TAG, "URI = " + uri);

        openFileAndDisplay(uri);
    }

    public void onModifyButtonClicked(View view) {
        if (mUri == null) {
            return;
        }
        try (OutputStream output = getContentResolver().openOutputStream(mUri, "wa")) {
            output.write(CONTENT_APPEND.getBytes());
        } catch (Exception e) {
            Log.e(LOG_TAG, "Failed to modify uri " + mUri + ": " + e);
            return;
        }
        openFileAndDisplay(mUri);
    }

    private void openFileAndDisplay(Uri uri) {
        try (InputStream input = getContentResolver().openInputStream(uri);
                Scanner sc = new Scanner(input)) {
            StringBuffer sb = new StringBuffer();
            while(sc.hasNext()) {
                sb.append(sc.nextLine());
            }

            mFileContentView.setText(sb.toString());
            Log.i(LOG_TAG, "File content = " + sb.toString());
        } catch (FileNotFoundException e) {
            Log.e(LOG_TAG, "Cannot open uri " + uri.toString() + ": " + e);
        } catch (IOException e) {
            Log.e(LOG_TAG, "Error reading uri " + uri.toString() + ": " + e);
        }
    }
}
