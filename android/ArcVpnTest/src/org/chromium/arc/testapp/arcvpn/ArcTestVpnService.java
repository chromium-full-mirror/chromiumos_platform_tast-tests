/*
 * Copyright 2022 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.arcvpn;

import android.R;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.content.Intent;
import android.net.VpnService;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.ParcelFileDescriptor;
import android.util.Log;

import java.io.IOException;

public class ArcTestVpnService extends VpnService {
    private static final String TAG = ArcTestVpnService.class.getSimpleName();

    // Metadata for the notification.
    private static final int NOTIFICATION_ID = 1;
    private static final String NOTIFICATION_CHANNEL_ID = TAG;

    // Saved as a member variable so the fd is seen as still being used. Otherwise it might get
    // closed from under us and also cause the tun interface to be closed as well.
    private ParcelFileDescriptor mTunFd;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        showNotification();
        setUpVpnService();

        return START_NOT_STICKY;
    }

    /** Called when the system has deactivated the underlying interface. */
    @Override
    public void onRevoke() {
        // Close and cleanup the fd so the service can be stopped.
        try {
            mTunFd.close();
        } catch (IOException e) {
            Log.w(TAG, "Unable to close tun fd.", e);
        }
        mTunFd = null;

        super.onRevoke();
    }

    /**
     * Creates the notification to be constantly shown while the service is running. This is needed
     * to be considered a proper foreground service.
     */
    private void showNotification() {
        getSystemService(NotificationManager.class)
                .createNotificationChannel(
                        new NotificationChannel(
                                NOTIFICATION_CHANNEL_ID,
                                NOTIFICATION_CHANNEL_ID,
                                NotificationManager.IMPORTANCE_NONE));

        startForeground(
                NOTIFICATION_ID,
                new Notification.Builder(this, NOTIFICATION_CHANNEL_ID)
                        .setSmallIcon(R.drawable.ic_dialog_info)
                        .build());
    }

    /** Registers ourselves as an actual VpnService and sets up the underlying interface. */
    private void setUpVpnService() {
        VpnService.prepare(getApplicationContext());

        mTunFd = new VpnService.Builder()
                .addAddress("192.168.2.2", 24)
                .addRoute("0.0.0.0", 0)
                .establish();
    }
}
