/*
 * Copyright 2024 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.devicepolicy;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.security.KeyChain;

public class KeyAccessChangedReceiver extends BroadcastReceiver {
    public static final String PREF_KEY_ALIAS = "keyAlias";

    @Override
    public void onReceive(Context context, Intent intent) {
        if (KeyChain.ACTION_KEY_ACCESS_CHANGED.equals(intent.getAction())) {

            String alias = intent.getStringExtra(KeyChain.EXTRA_KEY_ALIAS);

            // If key access is granted then save it in prefs so the test app can find it.
            // If key access is revoked then remove it from prefs.
            SharedPreferences.Editor editor = getSharedPreferences(context).edit();
            if (intent.getBooleanExtra(KeyChain.EXTRA_KEY_ACCESSIBLE, false)) {
                editor.putString(PREF_KEY_ALIAS, alias);
            } else if (alias.equals(getKeyAlias(context))) {
                editor.remove(PREF_KEY_ALIAS);
            }
            editor.apply();
        }
    }

    public static SharedPreferences getSharedPreferences(Context context) {
        return context.getSharedPreferences(
                KeyAccessChangedReceiver.class.getName(), Context.MODE_PRIVATE);
    }

    public static String getKeyAlias(Context context) {
        return getSharedPreferences(context).getString(PREF_KEY_ALIAS, null);
    }
}
