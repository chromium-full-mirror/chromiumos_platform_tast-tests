/*
 * Copyright 2020 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.windowmanager;

import android.app.ActivityManager;
import android.app.Fragment;
import android.os.Bundle;
import android.view.LayoutInflater;
import android.view.View;
import android.view.ViewGroup;
import android.widget.Button;
import android.content.pm.ActivityInfo;
import android.widget.CheckBox;

/**
 * A simple {@link Fragment} subclass.
 * It handles the logic (the controller) for "Options for Current Activity" like:
 * <ul>
 *     <li>changing orientation</li>
 *     <li>toggling immersive mode</li>
 * </ul>
 *
 * @see NewActivityFragment
 */
public class CurrentActivityFragment extends Fragment {
    public CurrentActivityFragment() {
        // Required empty public constructor
    }

    @Override
    public View onCreateView(
            LayoutInflater inflater, ViewGroup container, Bundle savedInstanceState) {
        // Inflate the layout for this fragment
        final View rootView =
                inflater.inflate(R.layout.fragment_current_activity, container, false);

        final Button buttonShow = rootView.findViewById(R.id.button_show);
        buttonShow.setOnClickListener(v -> {
            showSystemUI();
            ((BaseActivity) getActivity()).updateCaptionStatusView();
        });

        final Button buttonHide = rootView.findViewById(R.id.button_hide);
        buttonHide.setOnClickListener(v -> {
            final boolean enableSticky =
                    ((CheckBox) (rootView.findViewById(R.id.check_box_immersive_sticky)))
                            .isChecked();

            if (enableSticky) {
                hideSystemUISticky();
            } else {
                hideSystemUI();
            }
            ((BaseActivity) getActivity()).updateCaptionStatusView();
        });

        final Button buttonPortrait = rootView.findViewById(R.id.button_portrait);
        buttonPortrait.setOnClickListener(v -> getActivity().setRequestedOrientation(
                ActivityInfo.SCREEN_ORIENTATION_SENSOR_PORTRAIT));

        final Button buttonLandscape = rootView.findViewById(R.id.button_landscape);
        buttonLandscape.setOnClickListener(v -> getActivity().setRequestedOrientation(
                ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE));

        final Button buttonSensor = rootView.findViewById(R.id.button_sensor);
        buttonSensor.setOnClickListener(
                v -> getActivity().setRequestedOrientation(ActivityInfo.SCREEN_ORIENTATION_SENSOR));

        final Button buttonPin = rootView.findViewById(R.id.button_pin);
        buttonPin.setOnClickListener(
                view -> CurrentActivityFragment.this.getActivity().startLockTask());

        final Button buttonRefresh = rootView.findViewById(R.id.button_refresh);
        buttonRefresh.setOnClickListener(
                view -> ((BaseActivity)
                        CurrentActivityFragment.this.getActivity()).updateCaptionStatusView());

        return rootView;
    }

    // This snippet hides the system bars.
    // https://developer.android.com/training/system-ui/immersive.html
    private void hideSystemUI() {
        // Set the IMMERSIVE flag.
        // Set the content to appear under the system bars so that the content
        // doesn't resize when the system bars hide and show.
        final View decorView = getActivity().getWindow().getDecorView();
        decorView.setSystemUiVisibility(View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION // hide nav bar
                | View.SYSTEM_UI_FLAG_FULLSCREEN // hide status bar
                | View.SYSTEM_UI_FLAG_IMMERSIVE);
    }

    // This snippet hides the system bars. Sticky version.
    // https://developer.android.com/training/system-ui/immersive.html
    private void hideSystemUISticky() {
        final View decorView = getActivity().getWindow().getDecorView();
        decorView.setSystemUiVisibility(View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION | View.SYSTEM_UI_FLAG_FULLSCREEN
                | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);
    }

    // This snippet shows the system bars. It does this by removing all the flags
    // except for the ones that make the content appear under the system bars.
    // https://developer.android.com/training/system-ui/immersive.html
    // FIXME: Using SYSTEM_UI_FLAG_FULLSCREEN to prevent a might-be-an-Android bug
    // that doesn't display the Toolbar correctly when returning from immersive mode
    private void showSystemUI() {
        final View decorView = getActivity().getWindow().getDecorView();
        decorView.setSystemUiVisibility(
                View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN | View.SYSTEM_UI_FLAG_FULLSCREEN);
    }
}
