/*
 * Copyright 2023 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.arcvpn;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

public class VpnLaunchReceiver extends BroadcastReceiver {

    @Override
    public void onReceive(Context context, Intent intent) {
        context.startForegroundService(
                new Intent(context, ArcTestVpnService.class).putExtras(intent));
    }
}
