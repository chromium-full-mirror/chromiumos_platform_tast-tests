/*
 * Copyright 2023 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.arcshelficontest;

import android.app.Activity;
import android.app.ActivityManager;
import android.content.Intent;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.util.Log;
import android.widget.ImageView;
import android.widget.TextView;
import android.os.Bundle;

public class ShelfIconActivity extends Activity {

  private static final String TAG = "ArcShelfIconTest";

  @Override
  protected void onCreate(Bundle savedInstanceState) {
    super.onCreate(savedInstanceState);
    setContentView(R.layout.activity_main);

    String title = null;
    int iconId = R.drawable.ic_white;

    Intent intent = getIntent();
    if (intent != null) {
      title = intent.getStringExtra("title");
      String color = intent.getStringExtra("color");
      if ("red".equals(color)) {
        iconId = R.drawable.ic_red;
      } else if ("green".equals(color)) {
        iconId = R.drawable.ic_green;
      } else if ("blue".equals(color)) {
        iconId = R.drawable.ic_blue;
      }
    }
    Bitmap icon = BitmapFactory.decodeResource(getResources(), iconId);
    if (title == null) {
      title = "ArcShelfIconTestTitle";
    }

    ((ImageView)findViewById(R.id.imageView)).setImageResource(iconId);
    ((TextView) findViewById(R.id.windowtitle)).setText(title);

    setTaskDescription(new ActivityManager.TaskDescription(title, icon));
  }
}