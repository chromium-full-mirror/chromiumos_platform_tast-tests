/*
 * Copyright 2024 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.devicepolicy;

import android.app.Activity;
import android.app.admin.DevicePolicyManager;
import android.content.Context;
import android.content.pm.PackageManager.NameNotFoundException;
import android.os.Bundle;
import android.os.UserManager;
import android.util.Log;
import android.view.View;
import android.widget.Button;
import android.widget.Spinner;
import android.widget.TextView;

import java.io.IOException;
import java.security.KeyStore;
import java.security.KeyStoreException;
import java.security.NoSuchAlgorithmException;
import java.security.cert.CertificateException;
import java.security.cert.X509Certificate;
import java.util.Collections;
import java.util.Map;
import java.util.function.Supplier;

public class MainActivity extends Activity {
    private static final String TAG = "ArcDevicePolicyTest";
    private static final String POLICY_VALUE_KEY = "lstPolicies_value";
    private static final String OUTPUT_VALUE_KEY = "txtOutput_value";
    private static final String ERROR_VALUE_KEY = "txtError_value";
    private static final String TEST_CA_CERT_ISSUER =
            "CN=chromelab-wifi-testbed-root.mtv.google.com,L=Mountain View,ST=California,C=US";

    private TextView txtOutput;
    private Button btnTest;
    private Spinner lstPolicies;
    private TextView txtError;
    private Map<String, Supplier<Boolean>> arcPolicies;
    private DevicePolicyManager devicePolicyManager;
    private UserManager userManager;

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.main_activity);

        txtOutput = findViewById(R.id.txtOutput);
        btnTest = findViewById(R.id.btnTest);
        lstPolicies = findViewById(R.id.lstPolicies);
        txtError = findViewById(R.id.txtError);

        devicePolicyManager = (DevicePolicyManager) getSystemService(Context.DEVICE_POLICY_SERVICE);
        userManager = (UserManager) getSystemService(Context.USER_SERVICE);

        btnTest.setOnClickListener((View view) -> runTest());

        arcPolicies =
                Map.ofEntries(
                        Map.entry("ArcCertificatesSyncMode", this::isCaCertSyncDisabled),
                        isRestrictionUnapplied(
                                "credentialsConfigDisabled",
                                "credentialsConfig",
                                UserManager.DISALLOW_CONFIG_CREDENTIALS),
                        Map.entry("enabledSystemAppPackageNames", this::isSystemAppPackageDisabled),
                        isRestrictionUnapplied(
                                "installUnknownSourcesDisabled",
                                "installUnknownSources",
                                UserManager.DISALLOW_INSTALL_UNKNOWN_SOURCES),
                        isRestrictionUnapplied(
                                "modifyAccountsDisabled",
                                "modifyAccounts",
                                UserManager.DISALLOW_MODIFY_ACCOUNTS),
                        isRestrictionUnapplied(
                                "printingDisabled", "printing", UserManager.DISALLOW_PRINTING),
                        isRestrictionUnapplied(
                                "setWallpaper", "setWallpaper", UserManager.DISALLOW_SET_WALLPAPER),
                        isOperationAllowed(
                                "screenCaptureDisabled",
                                "screenshot",
                                () -> !devicePolicyManager.getScreenCaptureDisabled(null)),
                        isRestrictionUnapplied(
                                "shareLocationDisabled",
                                "shareLocation",
                                UserManager.DISALLOW_SHARE_LOCATION),
                        isOperationAllowed(
                                "cameraDisabled",
                                "camera",
                                () -> !devicePolicyManager.getCameraDisabled(null)),
                        isRestrictionUnapplied(
                                "unmuteMicrophoneDisabled",
                                "unmute",
                                UserManager.DISALLOW_UNMUTE_MICROPHONE),
                        isRestrictionUnapplied(
                                "vpnConfigDisabled", "vpnConfig", UserManager.DISALLOW_CONFIG_VPN));
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

        lstPolicies.setSelection(savedInstanceState.getInt(POLICY_VALUE_KEY));
        txtOutput.setText(savedInstanceState.getString(OUTPUT_VALUE_KEY));
        txtError.setText(savedInstanceState.getString(ERROR_VALUE_KEY));
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

    /*
        Returns a map entry where key is policy name recognized by ARC DPC and value is a function
        that can test whether the user is unrestricted to do the operation.

        @return True if the restriction is unapplied, False otherwise.
    */
    private Map.Entry<String, Supplier<Boolean>> isRestrictionUnapplied(
            String policy, String operation, String restrictionKey) {
        return isOperationAllowed(
                policy, operation, () -> !userManager.hasUserRestriction(restrictionKey));
    }

    /*
        Returns a map entry where key is policy name recognized by ARC DPC and value is a function
        that can test whether a policy restricted operation can be performed.

        @return True if the restricted operation can be performed, False otherwise.
    */
    private Map.Entry<String, Supplier<Boolean>> isOperationAllowed(
            String policy, String operation, Supplier<Boolean> operationTest) {
        return Map.entry(
                policy,
                () -> {
                    final boolean allowed = operationTest.get();
                    if (!allowed) {
                        logError(operation + " is disallowed", null);
                    }
                    return allowed;
                });
    }

    private boolean isCaCertSyncDisabled() {
        try {
            KeyStore store = KeyStore.getInstance("AndroidCAStore");
            if (store == null) {
                logError("Key store not found", null);
                return false;
            }
            store.load(/* stream= */ null, /* password= */ null);
            boolean syncDisabled = !isTestCertInstalled(store);
            if (!syncDisabled) {
                logError("Test cert found. Syncing is enabled", null);
            }
            return syncDisabled;
        } catch (IOException
                | KeyStoreException
                | NoSuchAlgorithmException
                | CertificateException e) {
            logError("Failed to load certs", e);
            return false;
        }
    }

    private static boolean isTestCertInstalled(KeyStore store) throws KeyStoreException {
        for (String alias : Collections.list(store.aliases())) {
            var cert = (X509Certificate) store.getCertificate(alias);
            if (cert.getIssuerX500Principal().getName().equals(MainActivity.TEST_CA_CERT_ISSUER)) {
                return true;
            }
        }
        return false;
    }

    private boolean isSystemAppPackageDisabled() {
        final String PACKAGE_NAME = "com.google.android.deskclock";
        try {
            final boolean enabled = getPackageManager().getApplicationInfo(PACKAGE_NAME, 0).enabled;
            if (enabled) {
                logError("System app is enabled", null);
            }
            return !enabled;
        } catch (NameNotFoundException e) {
            logError("System app not found", e);
            return true;
        }
    }

    private void logError(String message, Exception e) {
        Log.e(TAG, message, e);
        txtError.setText(message + (e == null ? "" : " " + e.toString()));
    }
}
