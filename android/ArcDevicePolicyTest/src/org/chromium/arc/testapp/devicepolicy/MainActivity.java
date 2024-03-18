/*
 * Copyright 2024 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.devicepolicy;

import android.app.Activity;
import android.os.Bundle;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.TextView;

public class MainActivity extends Activity {
  @Override
  public void onCreate(Bundle savedInstanceState) {
    super.onCreate(savedInstanceState);
    setContentView(R.layout.main_activity);

    final TextView txtOutput = findViewById(R.id.txtOutput);
    final Button btnTest = findViewById(R.id.btnTest);

    btnTest.setOnClickListener(
        (View view) -> {
          final EditText txtInput = findViewById(R.id.txtInput);
          txtOutput.setText(txtInput.getText().toString());
        });
  }
}
