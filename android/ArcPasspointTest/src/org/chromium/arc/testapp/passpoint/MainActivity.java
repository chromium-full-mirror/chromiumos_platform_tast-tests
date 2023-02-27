/*
 * Copyright 2023 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.passpoint;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.net.wifi.WifiManager;
import android.net.wifi.WifiNetworkSuggestion;
import android.net.wifi.hotspot2.ConfigParser;
import android.net.wifi.hotspot2.PasspointConfiguration;
import android.os.Bundle;
import android.view.View;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.List;

public class MainActivity extends Activity {
    private static final int LEGACY_API_REQUEST_CODE = 1;
    private static final int NETWORK_SUGGESTION_API_REQUEST_CODE = 2;

    // Content type for Android WiFi config, including Passpoint.
    // See https://source.android.com/devices/tech/connect/wifi-passpoint#download_mechanism
    private static final String TYPE_WIFI_CONFIG = "application/x-wifi-config";

    private WifiManager mWifiManager;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.main_activity);
        mWifiManager = (WifiManager) getSystemService(Context.WIFI_SERVICE);
    }

    private void addPasspointConfiguration(int apiType) {
        Intent chooseFile = new Intent(Intent.ACTION_GET_CONTENT);
        chooseFile.addCategory(Intent.CATEGORY_OPENABLE);
        chooseFile.setType(TYPE_WIFI_CONFIG);
        startActivityForResult(Intent.createChooser(chooseFile, "Choose a file"), apiType);
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (resultCode != RESULT_OK) {
            return;
        }
        try {
            PasspointConfiguration config = getPasspointConfiguration(data.getData());
            if (config == null) {
                return;
            }
            // Legacy API (available for Android SDK O+).
            // Deprecated for apps with Android SDK R+.
            if (requestCode == LEGACY_API_REQUEST_CODE) {
                mWifiManager.addOrUpdatePasspointConfiguration(config);
                return;
            }
            // Suggestion API (available for Android SDK Q+).
            final WifiNetworkSuggestion suggestion =
                    new WifiNetworkSuggestion.Builder().setPasspointConfig(config).build();
            List<WifiNetworkSuggestion> suggestions = new ArrayList<WifiNetworkSuggestion>();
            suggestions.add(suggestion);
            mWifiManager.addNetworkSuggestions(suggestions);
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    public void addPasspointLegacyAPI(View view) {
        addPasspointConfiguration(LEGACY_API_REQUEST_CODE);
    }

    public void addPasspointSuggestionAPI(View view) {
        addPasspointConfiguration(NETWORK_SUGGESTION_API_REQUEST_CODE);
    }

    PasspointConfiguration getPasspointConfiguration(Uri file) throws Exception {
        InputStream in = getContentResolver().openInputStream(file);
        ByteArrayOutputStream buffer = new ByteArrayOutputStream();
        int bytesRead;
        byte[] content = new byte[1024];
        while ((bytesRead = in.read(content, 0, content.length)) != -1) {
            buffer.write(content, 0, bytesRead);
        }
        buffer.flush();
        return ConfigParser.parsePasspointConfig(TYPE_WIFI_CONFIG, buffer.toByteArray());
    }
}
