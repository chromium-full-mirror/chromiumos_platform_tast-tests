/*
 * Copyright 2024 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.errornotificationtest;

import android.app.Activity;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.View;
import android.widget.Button;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends Activity {

    private Button anrButton;
    private Button crashButton;
    private ExecutorService executorService;
    private Handler mainHandler;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.main_activity);

        anrButton = findViewById(R.id.anrButton);
        crashButton = findViewById(R.id.crashButton);

        // Initialize ExecutorService with a single thread pool
        executorService = Executors.newSingleThreadExecutor();
        // Handler to post back to the main thread
        mainHandler = new Handler(Looper.getMainLooper());

        // Set click listeners for both buttons
        anrButton.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                triggerANR();
            }
        });

        crashButton.setOnClickListener(new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                triggerCrash();
            }
        });
    }

    // Method to trigger an ANR by sleeping on the main thread
    private void triggerANR() {
        try {
            // This will block the main thread for 100 seconds
            Thread.sleep(100000);
        } catch (InterruptedException e) {
            e.printStackTrace();
        }
    }

    // Method to trigger a crash after 5 seconds using multithreading
    private void triggerCrash() {
        executorService.execute(new Runnable() {
            @Override
            public void run() {
                try {
                    // Sleep for 5 seconds
                    Thread.sleep(5000);
                    // Trigger a runtime exception
                    throw new RuntimeException("This is a forced crash after 5 seconds!");
                } catch (InterruptedException e) {
                    e.printStackTrace();
                }
            }
        });
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        // Shutdown the executor service when the activity is destroyed
        executorService.shutdown();
    }
}
