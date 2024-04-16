/*
 * Copyright 2024 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.devicepolicy;

import android.app.Activity;
import android.app.WallpaperManager;
import android.app.admin.DevicePolicyManager;
import android.content.Context;
import android.os.Bundle;
import android.util.Log;
import android.view.View;
import android.widget.Button;
import android.widget.Spinner;
import android.widget.TextView;

import java.io.IOException;
import java.util.HashMap;
import java.util.Map;
import java.util.function.Supplier;

public class MainActivity extends Activity {
    private static final String TAG = "ArcDevicePolicyTest";
    private static final String POLICY_VALUE_KEY = "lstPolicies_value";
    private static final String OUTPUT_VALUE_KEY = "txtOutput_value";
    private static final String ERROR_VALUE_KEY = "txtError_value";

    private TextView txtOutput;
    private Button btnTest;
    private Spinner lstPolicies;
    private TextView txtError;
    private Map<String, Supplier<Boolean>> arcPolicies;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.main_activity);

        txtOutput = findViewById(R.id.txtOutput);
        btnTest = findViewById(R.id.btnTest);
        lstPolicies = findViewById(R.id.lstPolicies);
        txtError = findViewById(R.id.txtError);

        btnTest.setOnClickListener((View view) -> runTest());

        arcPolicies =
                new HashMap<>() {
                    {
                        put("setWallpaper", () -> setWallpaper());
                        put("cameraDisabled", () -> getCameraDisabled());
                    }
                };
    }

    @Override
    public void onSaveInstanceState(Bundle outState) {
        super.onSaveInstanceState(outState);

        outState.putInt(POLICY_VALUE_KEY, lstPolicies.getSelectedItemPosition());
        outState.putString(OUTPUT_VALUE_KEY, txtOutput.getText().toString());
        outState.putString(ERROR_VALUE_KEY, txtError.getText().toString());
    }

    @Override
    public void onRestoreInstanceState(Bundle savedInstanceState) {
        super.onRestoreInstanceState(savedInstanceState);

        if (savedInstanceState != null) {
            lstPolicies.setSelection(savedInstanceState.getInt(POLICY_VALUE_KEY));
            txtOutput.setText(savedInstanceState.getString(OUTPUT_VALUE_KEY));
            txtError.setText(savedInstanceState.getString(ERROR_VALUE_KEY));
        }
    }

    private void runTest() {
        final String policy = lstPolicies.getSelectedItem().toString();
        txtOutput.setText("");
        txtError.setText("");

        boolean result;
        if (arcPolicies.containsKey(policy)) {
            result = arcPolicies.get(policy).get();
        } else {
            logError("Unrecognized policy: " + policy, null);
            result = false;
        }
        txtOutput.setText(String.valueOf(result));
    }

    private Boolean getCameraDisabled() {
        final var dpm = (DevicePolicyManager) getSystemService(Context.DEVICE_POLICY_SERVICE);
        final boolean isCameraDisabled = dpm.getCameraDisabled(null);
        if (isCameraDisabled) {
            logError("Camera is disabled", null);
        }
        return !isCameraDisabled;
    }

    private Boolean setWallpaper() {
        final WallpaperManager manager = WallpaperManager.getInstance(getApplicationContext());
        if (!manager.isSetWallpaperAllowed()) {
            logError("Changing wallpaper not allowed", null);
            return false;
        }

        final int previousId = manager.getWallpaperId(WallpaperManager.FLAG_SYSTEM);
        try {
            manager.setResource(R.drawable.wallpaper);
        } catch (IOException e) {
            logError("Failed to set wallpaper", e);
            return false;
        }
        final int currentId = manager.getWallpaperId(WallpaperManager.FLAG_SYSTEM);
        if (previousId == currentId) {
            logError("Wallpaper did not change", null);
        }
        return previousId != currentId;
    }

    private void logError(String message, Exception e) {
        Log.e(TAG, message, e);
        txtError.setText(message + (e == null ? "" : " " + e.toString()));
    }
}
