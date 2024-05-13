/*
 * Copyright 2024 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.localechanger;

import android.app.Activity;
import android.app.LocaleManager;
import android.os.Bundle;
import android.os.LocaleList;
import android.util.Log;
import android.view.View;
import android.widget.TextView;

import java.util.Locale;

/**
 * Main activity that allows the user to select a color to change the System UI. There are three
 * color buttons displayed to the user: green, red, and blue. The System UI will change to any of
 * the three colors that the user selects.
 */
public class MainActivity extends Activity {
    private static final String TAG = "ArcLocaleChanger";
    private LocaleManager mLocaleManager;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.main_activity);
        mLocaleManager = getSystemService(LocaleManager.class);
        TextView mStatusTextView = findViewById(R.id.locale_text);
        Locale mSystemLocale = Locale.getDefault();
        Locale appLocale = getResources().getConfiguration().getLocales().get(0);
        mStatusTextView.setText(
                getString(R.string.current_locale_text, appLocale));
    }


    /**
     * Changes the system ui based on the which button was pressed.
     */
    public void changeLocale(View view) {
        final Locale newLocale;
        switch (view.getId()) {
            case R.id.spanish_button_text:
                newLocale = new Locale("es", "US");
                break;
            case R.id.english_button_text:
                newLocale = new Locale("en", "US");
                break;
            case R.id.french_button_text:
                newLocale = new Locale("fr", "FR");
                break;
            default:
                // System Default language.
                newLocale = null;
        }
        mLocaleManager.setApplicationLocales(
                newLocale == null ? LocaleList.getEmptyLocaleList() : new LocaleList(newLocale));
        Log.i(TAG, "Settings application locale to: " + newLocale);
    }
}
