/*
 * Copyright 2023 The ChromiumOS Authors
 * Use of this source code is governed by a BSD-style license that can be
 * found in the LICENSE file.
 */

package org.chromium.arc.testapp.qos_monitor;

import static android.system.OsConstants.AF_INET;
import static android.system.OsConstants.AF_INET6;
import static android.system.OsConstants.IPPROTO_IPV6;
import static android.system.OsConstants.IPV6_V6ONLY;
import static android.system.OsConstants.SOCK_DGRAM;
import static android.system.OsConstants.SOCK_STREAM;

import android.app.Activity;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.media.MediaRecorder;
import android.os.Bundle;
import android.system.ErrnoException;
import android.system.Os;
import android.util.Log;
import java.io.File;
import java.io.FileDescriptor;
import java.io.IOException;
import java.net.InetAddress;
import java.net.SocketException;
import java.net.UnknownHostException;
import java.nio.ByteBuffer;

/**
 * Test Activity for the arc.QosAppMonitor Tast test.
 *
 * <p> This test app can be used to setup TCP IPv4, TCP IPv6 and UDP IPv4, UDP IPv6 sockets and
 * send messages through those sockets.
 *
 * <p> The sockets are set using adb command:
 * adb shell am broadcast -a org.chromium.arc.testapp.qos_monitor.SETUP_SOCKET --es proto <PROTO>
 * --es address <ADDRESS> --ei port <PORT>
 * When proto is specified as IPv6 (`udp6` or `tcp6`) but IPv4 address is passed in, an IPv6 socket
 * with IPv4-mapped IPv6 address will be created.
 *
 * <p> The messages are sent using adb command:
 * adb shell am broadcast -a org.chromium.arc.testapp.qos_monitor.SEND_SOCKET_MESSAGE --es
 * message <MESSAGE>
 * The message will be sent from the most recently setup socket.
 *
 * <p> This app can also start and stop recording audio by adb cmd:
 * adb shell am broadcast -a org.chromium.arc.testapp.qos_monitor.START_RECORDING
 * adb shell am broadcast -a org.chromium.arc.testapp.qos_monitor.STOP_RECORDING
 * This can simulate that the user is during a web conference when productivity app is being used.
 */
public class QosMonitorActivity extends Activity {
    private final String TAG = QosMonitorActivity.class.getSimpleName();

    // Intent for setting up socket.
    private static final String SETUP_SOCKET =
            "org.chromium.arc.testapp.qos_monitor.SETUP_SOCKET";

    // Intent for sending socket message.
    private static final String SEND_SOCKET_MESSAGE =
            "org.chromium.arc.testapp.qos_monitor.SEND_SOCKET_MESSAGE";

    // Intent for starting recording.
    private static final String START_RECORDING =
            "org.chromium.arc.testapp.qos_monitor.START_RECORDING";

    // Intent for stopping recording.
    private static final String STOP_RECORDING =
            "org.chromium.arc.testapp.qos_monitor.STOP_RECORDING";

    // Keys used for setting intent.
    private static final String PROTOCOL_KEY = "proto";
    private static final String MESSAGE_KEY = "message";
    private static final String ADDRESS_KEY = "address";
    private static final String PORT_KEY = "port";

    // Values used for protocol intent key.
    private static final String PROTOCOL_TCP_IPV6 = "tcp6";
    private static final String PROTOCOL_TCP_IPV4 = "tcp4";
    private static final String PROTOCOL_UDP_IPV6 = "udp6";
    private static final String PROTOCOL_UDP_IPV4 = "udp4";

    // Destination IP address, port and protocol used to setup socket.
    private String mAddress;
    private int mPort;

    // File descriptor for the setup socket.
    private FileDescriptor mFd;

    // Media recorder to start/stop recording audio. This is used to simulate a web conference
    // when productivity/social apps are being used.
    private MediaRecorder mRecorder;

    // Path to save temporary recorded audio file. This file will be deleted automatically after
    // the activity is destroyed.
    private String mFileName;

    public class QosBroadcastReceiver extends BroadcastReceiver {
            private static final String TAG = "QosBroadcastReceiver";
            @Override
            public void onReceive(Context context, Intent intent) {
                // Setup socket by given protocol, address and port.
                if (SETUP_SOCKET.equals(intent.getAction())) {
                    String proto = intent.getStringExtra(PROTOCOL_KEY);
                    mAddress = intent.getStringExtra(ADDRESS_KEY);
                    if (mAddress == null) {
                        Log.e(TAG, "Address is not correctly set, setup socket failed.");
                        return;
                    }
                    mPort = intent.getIntExtra(PORT_KEY, -1);
                    if (mPort == -1) {
                        Log.e(TAG, "port is invalid, setup socket failed.");
                        return;
                    }
                    switch (proto) {
                        case PROTOCOL_TCP_IPV4:
                            setupSocket(AF_INET, SOCK_STREAM);
                            return;
                        case PROTOCOL_TCP_IPV6:
                            setupSocket(AF_INET6, SOCK_STREAM);
                            return;
                        case PROTOCOL_UDP_IPV4:
                            setupSocket(AF_INET, SOCK_DGRAM);
                            return;
                        case PROTOCOL_UDP_IPV6:
                            setupSocket(AF_INET6, SOCK_DGRAM);
                            return;
                        default:
                            Log.e(TAG, "Invalid procotol: " + proto);
                            return;
                    }
                }
                // Send out socket message through last setup socket.
                if (SEND_SOCKET_MESSAGE.equals(intent.getAction())) {
                    String message = intent.getStringExtra(MESSAGE_KEY);
                    if (message == null) {
                        Log.e(TAG, "Message is not correctly set, send message failed.");
                        return;
                    }
                    sendMessage(message);
                }
                if (START_RECORDING.equals(intent.getAction())) {
                    startRecording();
                    return;
                }
                if (STOP_RECORDING.equals(intent.getAction())) {
                    stopRecording();
                    return;
                }
            }
        }
    private BroadcastReceiver mReceiver  = new QosBroadcastReceiver();

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        mFileName = getExternalCacheDir().getAbsolutePath() + "/qosmonitor.3gp";

        IntentFilter filter = new IntentFilter();
        filter.addAction(SEND_SOCKET_MESSAGE);
        filter.addAction(SETUP_SOCKET);
        filter.addAction(START_RECORDING);
        filter.addAction(STOP_RECORDING);
        registerReceiver(mReceiver, filter);
    }

    /**
     * Set up a socket. Address and port for the socket are passed in by intent extra values.
     *
     * @param family IP family of the socket to setup, one of {@code AF_INET} or {@code AF_INET}.
     * If IP family is set as AF_INET6 but IPv4 address is provided, the socket is setup with
     * IPv4-mapped IPv6 address.
     * @param type type of the socket to setup, one of {@code SOCK_STREAM} for TCP socket, or
     * {@code SOCK_DGRAM} for UDP socket.
     */
    private void setupSocket(int family, int type) {
        new Thread(() -> {
            InetAddress target;
            try {
                target = InetAddress.getByName(mAddress);
            } catch (UnknownHostException e) {
                Log.e(TAG, "Invalid address: ", e);
                return;
            }
            try {
                mFd = Os.socket(family, type, 0);
                // For IPv6 address, set IPV6_V6ONLY value to 0 to allow it to accept both IPv6
                // address and IPv4 address.
                if (family == AF_INET6) {
                    Os.setsockoptInt(mFd, IPPROTO_IPV6, IPV6_V6ONLY, 0);
                }
                Os.connect(mFd, target, mPort);
            } catch (ErrnoException | SocketException e) {
                Log.e(TAG, "Failed to open and connect socket: ", e);
                try {
                    if (mFd != null && mFd.valid()) {
                        Os.close(mFd);
                    }
                } catch (ErrnoException err) {
                    Log.e(TAG, "Failed to close opened socket: ", e);
                    return;
                }
            }
        }).start();
    }

    /**
     * Send a packet through latest setup socket. Message for the payload of this packet is passed
     * in by intent extra value.
     *
     * @param message payload of the message to be sent.
     */
    private void sendMessage(String message) {
        if (mFd == null || !mFd.valid()) {
            Log.e(TAG, "File descriptor is not valid, send message failed");
            return;
        }

        new Thread(() -> {
            try {
                ByteBuffer buffer = ByteBuffer.wrap(message.getBytes());
                Os.sendto(mFd, buffer, /* flags= */0,
                        InetAddress.getByName(mAddress), mPort);
            } catch (UnknownHostException |  ErrnoException | SocketException e) {
                Log.e(TAG, "Send message failed", e);
            }
        }).start();
    }

    /**
     * Start recording. This can simulate that the user started a web conference when
     * test app is productivity/social app.
     */
    private void startRecording() {
        mRecorder = new MediaRecorder();
        mRecorder.setAudioSource(MediaRecorder.AudioSource.MIC);
        mRecorder.setOutputFormat(MediaRecorder.OutputFormat.THREE_GPP);
        mRecorder.setOutputFile(mFileName);
        mRecorder.setAudioEncoder(MediaRecorder.AudioEncoder.AMR_NB);
        try {
            mRecorder.prepare();
        } catch (IOException e) {
            Log.e(TAG, "Media recorder prepare failed", e);
            return;
        }
        mRecorder.start();
    }

    /**
     * Stop recording. This can simulate that the user ended a web conference when
     * test app is productivity/social app.
     */
    private void stopRecording() {
        if (mRecorder == null) {
            return;
        }
        mRecorder.stop();
        mRecorder.release();
        mRecorder = null;

        File recordFile = new File(mFileName);
        if (recordFile.exists()) {
            recordFile.delete();
        }
    }

    @Override
    public void onDestroy(){
        super.onDestroy();
        stopRecording();
        try {
            if (mFd != null && mFd.valid()) {
                Os.close(mFd);
            }
        } catch (ErrnoException e) {
            Log.e(TAG, "failed to close file descriptor", e);
        }
    }
}
