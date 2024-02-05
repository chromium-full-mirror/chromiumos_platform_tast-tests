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
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkInfo;
import android.net.VpnService;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.ParcelFileDescriptor;
import android.util.Log;

import java.io.IOException;
import java.io.OutputStream;
import java.io.PrintWriter;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.UnknownHostException;

public class ArcTestVpnService extends VpnService {
    private static final String TAG = ArcTestVpnService.class.getSimpleName();

    // Intent for setting up a socket.
    private static final String SETUP_SOCKET =
            "org.chromium.arc.testapp.arcvpn.SETUP_SOCKET";
    // Intent for sending a message through the last set up socket.
    private static final String SEND_MESSAGE =
            "org.chromium.arc.testapp.arcvpn.SEND_MESSAGE";

    // Keys used for setting intent.
    private static final String PROTOCOL_KEY = "proto";
    private static final String MESSAGE_KEY = "message";
    private static final String ADDRESS_KEY = "address";
    private static final String PORT_KEY = "port";

    // Values used for protocol intent key.
    // These fields are in sync with l4server.Family in:
    // platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server/l4server.go
    private static final String PROTOCOL_TCP = "tcp";
    private static final String PROTOCOL_UDP = "udp";

    // Metadata for the notification.
    private static final int NOTIFICATION_ID = 1;
    private static final String NOTIFICATION_CHANNEL_ID = TAG;

    // Saved as a member variable so the fd is seen as still being used. Otherwise it might get
    // closed from under us and also cause the tun interface to be closed as well.
    private ParcelFileDescriptor mTunFd;

    // Broadcast receiver to receive intent to setup sockets or send messages.
    private ArcVpnBroadcastReceiver mBroadcastReceiver;

    // Address and port used to setup socket.
    private InetAddress mAddress;
    private int mPort;

    // Variables that are used to connect to a TCP server and write messages to the setup TCP
    // sockets.
    private Socket mTcpSocket;
    private PrintWriter mWriter;

    // UDP socket that is used to connect to a UDP server and write messages to this socket.
    private DatagramSocket mUdpSocket;

    // Last setup socket family. Used to send message from last setup socket.
    private String mLastSetupSocketFamily;

    public class ArcVpnBroadcastReceiver extends BroadcastReceiver {
        private static final String TAG = "ArcVpnBroadcastReceiver";

        @Override
        public void onReceive(Context context, Intent intent) {
            // Setup socket by given protocol, address and port.
            if (SETUP_SOCKET.equals(intent.getAction())) {
                String proto = intent.getStringExtra(PROTOCOL_KEY);
                String address = intent.getStringExtra(ADDRESS_KEY);
                if (address == null) {
                    Log.e(TAG, "Address is not correctly set, setup socket failed.");
                    return;
                }
                try {
                    mAddress = InetAddress.getByName(address);
                } catch (UnknownHostException e) {
                    Log.e(TAG, "Address is unknown, setup socket failed..");
                    return;
                }
                mPort = intent.getIntExtra(PORT_KEY, -1);
                if (mPort == -1) {
                    Log.e(TAG, "Port is invalid, setup socket failed..");
                    return;
                }
                switch (proto) {
                    case PROTOCOL_TCP:
                        setupTcpSocket();
                        break;
                    case PROTOCOL_UDP:
                        setupUdpSocket();
                        break;
                    default:
                        Log.e(TAG, "Invalid procotol: " + proto + ", setup socket failed.");
                        break;
                }
            } else if (SEND_MESSAGE.equals(intent.getAction())) {
                String message = intent.getStringExtra(MESSAGE_KEY);
                if (message == null) {
                    Log.e(TAG, "Message is not correctly set, send message failed.");
                    return;
                }
                switch (mLastSetupSocketFamily) {
                    case PROTOCOL_TCP:
                        sendTcpMessage(message);
                        break;
                    case PROTOCOL_UDP:
                        sendUdpMessage(message);
                        break;
                    default:
                        Log.e(TAG, "Socket has not been setup yet, send message failed.");
                        break;
                }
            }
        }
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        showNotification();
        setUpVpnService();

        mBroadcastReceiver = new ArcVpnBroadcastReceiver();
        IntentFilter intentFilter = new IntentFilter();
        intentFilter.addAction(SETUP_SOCKET);
        intentFilter.addAction(SEND_MESSAGE);
        registerReceiver(mBroadcastReceiver, intentFilter);

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
        unregisterReceiver(mBroadcastReceiver);
        try {
            if (mWriter != null) {
                mWriter.close();
            }
            if (mTcpSocket != null && !mTcpSocket.isClosed()) {
                mTcpSocket.close();
            }
            if (mUdpSocket != null && !mUdpSocket.isClosed()) {
                mUdpSocket.close();
            }
        } catch (IOException e) {
            Log.w(TAG, "Failed to close sockets", e);
        }
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

    /**
     * Sets up a TCP socket using given address and port of the remote peer we want to connect to.
     * Currently the setup only works with the situation that there is only one connected WiFi
     * network on the device and only WiFi is supported.
     * When called multiple times in one test, the older socket will be replaced by newly setup
     * socket for sending messages.
     */
    private void setupTcpSocket() {
        new Thread(() -> {
                try {
                    Network net = getConnectedWifiNetwork();
                    if (net == null) {
                        Log.e(TAG, "A connected WiFi network does not exist, set up socket"
                                + "failed.");
                        return;
                    }
                    mTcpSocket = net.getSocketFactory().createSocket();
                    protect(mTcpSocket);
                    mTcpSocket.connect(new InetSocketAddress(mAddress, mPort));
                    mWriter = new PrintWriter(mTcpSocket.getOutputStream(), /*autoFlush=*/ true);
                    mLastSetupSocketFamily = PROTOCOL_TCP;
                } catch (IOException e) {
                    Log.e(TAG, "Error opening TCP socket", e);
                }
        }).start();
    }

    /**
     * Sends a message via setup TCP socket. This method needs to be called after calling
     * {@link #setupTcpSocket()}.
     */
    public void sendTcpMessage(String msg) {
        new Thread(() -> {
            try {
                if (mWriter == null) {
                    Log.e(TAG, "TCP socket has not been set up yet, send message failed.");
                    return;
                }
                mWriter.println(msg);
            } catch (Exception e) {
                Log.e(TAG, "Failed to send TCP messages", e);
            }
        }).start();
    }

    /**
     * Sets up a UDP socket using given address and port of the remote peer we want to connect to.
     * Currently the setup only works with the situation that there is only one connected WiFi
     * network on the device and only WiFi is supported.
     * When called multiple times in one test, the older socket will be replaced by newly setup
     * socket for sending messages.
     */
    private void setupUdpSocket() {
        new Thread(() -> {
            try {
                Network net = getConnectedWifiNetwork();
                if (net == null) {
                    Log.e(TAG, "A connected WiFi network that satisfies requirement does not "
                            + "exist, set up socket failed.");
                    return;
                }
                mUdpSocket = new DatagramSocket();
                net.bindSocket(mUdpSocket);
                protect(mUdpSocket);
                mUdpSocket.connect(mAddress, mPort);
                mLastSetupSocketFamily = PROTOCOL_UDP;
            } catch (IOException e) {
                Log.e(TAG, "Error opening UDP socket", e);
            }
        }).start();
    }

    /**
     * Sends a message via setup UDP socket. This method needs to be called after calling
     * {@link #setupUdpSocket()}.
     */
    public void sendUdpMessage(String msg) {
        new Thread(() -> {
            try {
                if (mUdpSocket == null) {
                    Log.e(TAG, "UDP socket has not been set up yet, send message failed.");
                    return;
                }
                DatagramPacket dp = new DatagramPacket(msg.getBytes(), msg.length(),
                        mAddress, mPort);
                mUdpSocket.send(dp);
            } catch (IOException e) {
                Log.e(TAG, "Failed to send UDP messages", e);
            }
        }).start();
    }

    /**
     * Gets a connected WiFi network from all available networks. It is expected that there is
     * only one such network based on the test setup.
     *
     * @return WiFi network, null if it doesn't exist
     */
    private Network getConnectedWifiNetwork() {
        ConnectivityManager connectivityManager = (ConnectivityManager) getApplicationContext()
                .getSystemService(Context.CONNECTIVITY_SERVICE);
        if (connectivityManager == null) {
            Log.e(TAG, "Get connected WiFi network failed, failed to get ConnectivityManager.");
            return null;
        }
        Network[] networks = connectivityManager.getAllNetworks();
        for (Network network : networks) {
            NetworkInfo networkInfo = connectivityManager.getNetworkInfo(network);
            if (networkInfo != null && networkInfo.getType() ==
                    ConnectivityManager.TYPE_WIFI &&
                    networkInfo.isConnected()) {
                return network;
            }
        }
        return null;
    }
}
