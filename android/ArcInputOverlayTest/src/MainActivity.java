/*
 * Copyright 2022 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.inputoverlay;

import android.app.Activity;
import android.os.Bundle;
import android.os.SystemClock;
import android.util.Log;
import android.view.InputDevice;
import android.view.InputEvent;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.widget.Button;
import android.widget.EditText;
import android.widget.ListView;
import android.widget.TextView;

import org.json.JSONArray;
import org.json.JSONException;

import java.util.ArrayList;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

public class MainActivity extends Activity {
  final static String TAG = "InputOverlayTest";

  private Button mButton;
  private EditText mEdit;
  private ListView mList;
  private EventListAdapter mAdapter;
  private TextView mEvents;
  private TextView mCount;
  private ExecutorService mExecutor = Executors.newSingleThreadExecutor();
  private ArrayList<ReceivedEvent> mRecvEvents = new ArrayList<>();
  private View mView;

  @Override
  protected void onCreate(Bundle savedInstanceState) {
    super.onCreate(savedInstanceState);
    setContentView(R.layout.activity_main);

    mEdit = findViewById(R.id.m_edit);
    mButton = findViewById(R.id.m_button);
    mList = findViewById(R.id.m_list);
    mAdapter = new EventListAdapter(getApplicationContext(), mRecvEvents);
    mList.setAdapter(mAdapter);
    mEvents = findViewById(R.id.event_json);
    mCount = findViewById(R.id.event_count);
    mView = findViewById(R.id.main_view);
  }

  @Override
  public boolean dispatchTouchEvent(MotionEvent ev) {
    printAndDisplayEvent(ev);
    traceEvent(new ReceivedEvent(ev, SystemClock.elapsedRealtimeNanos()));
    // Stop dispatching gamepad event.
    if (isGameEvent(ev)) {
      return true;
    }
    return super.dispatchTouchEvent(ev);
  }

  @Override
  public boolean dispatchKeyEvent(KeyEvent ev) {
    printAndDisplayEvent(ev);
    if (ev.getKeyCode() == KeyEvent.KEYCODE_ESCAPE) {
        finishTrace();
        return false;
    }
    if (ev.getKeyCode() == KeyEvent.KEYCODE_DEL) {
        clearUI();
        return false;
    }
    // Stop dispatching gamepad event.
    if (isGameEvent(ev)) {
      return true;
    }
    return super.dispatchKeyEvent(ev);
  }

  @Override
  public boolean dispatchGenericMotionEvent(MotionEvent ev) {
    printAndDisplayEvent(ev);
    // Stop dispatching gamepad event.
    if (isGameEvent(ev)) {
      return true;
    }
    return super.dispatchGenericMotionEvent(ev);
  }

  @Override
  public boolean dispatchTrackballEvent(MotionEvent ev) {
    printAndDisplayEvent(ev);
    return super.dispatchTrackballEvent(ev);
  }

  // Finish trace and save the events as JSON to TextView UI.
  private void finishTrace() {
      // Serialize events to JSON
      mExecutor.submit(
              () -> {
                  JSONArray arr = new JSONArray();
                  try {
                      for (ReceivedEvent ev : mRecvEvents) {
                          arr.put(ev.toJSON());
                      }
                      String json = arr.toString();
                      int len = arr.length();
                      runOnUiThread(() -> setEvents(json, len));
                  } catch (JSONException e) {
                      Log.e(TAG, "Unable to serialize events to JSON: " + e.getMessage());
                  }
              });
  }

  private void clearUI() {
      mRecvEvents.clear();
      mAdapter.notifyDataSetChanged();
      mExecutor.submit(
              () -> {
                  runOnUiThread(() -> setEvents("", 0));
              });
  }

  private void printAndDisplayEvent(InputEvent event) {
    Log.v(TAG, event.toString());
  }

  private boolean isGameEvent(InputEvent event) {
    final int source = event.getSource();
    if ((source & (InputDevice.SOURCE_GAMEPAD|InputDevice.SOURCE_JOYSTICK)) != 0) {
      return true;
    }
    return false;
  }

  /** Called to record timing of received input events. */
  private void traceEvent(ReceivedEvent recv) {
      // Ignore TouchScreen event that is ACTION_CANCEL
      if (recv.source == "Touchscreen" && recv.action == "ACTION_CANCEL") {
          return;
      }

      mRecvEvents.add(recv);
      mAdapter.notifyDataSetChanged();

      // Record the event numbers.
      mExecutor.submit(
              () -> {
                  runOnUiThread(() -> setEvents("", mRecvEvents.size()));
              });
  }

  private void setEvents(String json, Integer count) {
      mEvents.setText(json);
      mCount.setText(count.toString());
  }
}