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
import android.net.IpPrefix;
import android.net.LinkProperties;
import android.net.Network;
import android.net.VpnService;
import android.os.Build.VERSION;
import android.os.Build.VERSION_CODES;
import android.os.ParcelFileDescriptor;
import android.util.Log;

import android.util.Pair;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.io.PrintWriter;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.Inet4Address;
import java.net.Inet6Address;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.UnknownHostException;
import java.nio.ByteBuffer;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class ArcTestVpnService extends VpnService {
    private static final String TAG = ArcTestVpnService.class.getSimpleName();

    // Intent for sending a message through the last set up socket.
    private static final String SEND_MESSAGE =
            "org.chromium.arc.testapp.arcvpn.SEND_MESSAGE";

    // Keys used for setting intent extras for setting up VPN service.
    private static final String OVERLAY_ADDRESSES_KEY = "overlay_addresses";
    private static final String DNS_SERVER_KEY = "dns_server";
    private static final String INCLUDED_ROUTES_KEY = "included_routes";
    private static final String EXCLUDED_ROUTES_KEY = "excluded_routes";
    private static final String MTU_KEY = "mtu";
    // Keys used for setting intent extras for connecting to toy VPN server.
    private static final String INTERFACE_KEY = "interface";
    private static final String ADDRESS_KEY = "address";
    private static final String PORT_KEY = "port";
    // Keys used for setting intent extras for setting up test socket and sending messages to this
    // socket.
    private static final String SOCKET_PROTOCOL_KEY = "sockproto";
    private static final String SOCKET_INTERFACE_KEY = "sockinterface";
    private static final String SOCKET_ADDRESS_KEY = "sockaddress";
    private static final String SOCKET_PORT_KEY = "sockport";
    private static final String MESSAGE_KEY = "message";

    // Values used for protocol intent key.
    // These fields are in sync with l4server.Family in:
    // platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/network/virtualnet/l4server/l4server.go
    private static final String PROTOCOL_TCP = "tcp";
    private static final String PROTOCOL_UDP = "udp";

    // The default overlay address installed onto the TUN interface, if it's not specified in the
    // intent for launching the VPN service.
    private static final String DEFAULT_OVERLAY_ADDRESS = "192.168.2.2";

    // Useful so ARC doesn't use the host's DNS servers as a fallback. This shouldn't functionally
    // change the VPN behavior, but may affect ARC's networking behavior on syncing host->ARC DNS
    // servers.
    private static final String DEFAULT_DNS_SERVER = "8.8.8.8";

    // The default MTU value on the TUN interface. This is the default but usually not a good enough
    // value for a VPN interface.
    private static final int DEFAULT_MTU = 1500;

    // The header size of a message in the protocol of the toy VPN server. See the package doc in
    // platform/tast-tests/src/go.chromium.org/tast-tests/cros/local/network/vpn/internal/toyserver
    // for more details.
    private static final int TOY_VPN_MESSAGE_HEADER_SIZE = 4;

    // Metadata for the notification.
    private static final int NOTIFICATION_ID = 1;
    private static final String NOTIFICATION_CHANNEL_ID = TAG;

    // Invalid port number.
    private static final int INVALID_PORT = -1;

    // Saved as a member variable so the fd is seen as still being used. Otherwise it might get
    // closed from under us and also cause the tun interface to be closed as well.
    private ParcelFileDescriptor mTunFd;

    // Broadcast receiver to receive intent to setup sockets or send messages.
    private ArcVpnBroadcastReceiver mBroadcastReceiver;

    // Variables that are used to connect to a TCP server and write messages to the setup TCP
    // sockets.
    private Socket mTcpSocket;
    private PrintWriter mWriter;

    // UDP socket that is used to connect to a UDP server and write messages to this socket.
    private DatagramSocket mUdpSocket;

    // Last setup socket family. Used to send message from last setup socket.
    private String mLastSetupSocketFamily;

    // A separate worker thread to sequentialize the one-off tasks.
    ExecutorService mExecutor = Executors.newSingleThreadExecutor();

    public class ArcVpnBroadcastReceiver extends BroadcastReceiver {
        private static final String TAG = "ArcVpnBroadcastReceiver";

        @Override
        public void onReceive(Context context, Intent intent) {
            if (SEND_MESSAGE.equals(intent.getAction())) {
                String message = intent.getStringExtra(MESSAGE_KEY);
                if (message == null) {
                    Log.e(TAG, "Message is not correctly set, send message failed.");
                    return;
                }
                if (mLastSetupSocketFamily == null) {
                    Log.e(TAG, "Socket has not been setup yet, send message failed.");
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
                }
            }
        }
    }

    /**
     * Parses a CIDR string, e.g., "192.168.0.1/24" -> ("192.168.0.1", 24).
     * Note that there is a IpPrefix class in Android which provides the same functionality, but
     * it's only available on T+.
     */
    private static Pair<InetAddress, Integer> parseIpCidrString(String cidr) {
        String[] parts = cidr.split("/");
        if (parts.length == 0 || parts.length > 2) {
            throw new IllegalArgumentException("Invalid CIDR string " + cidr);
        }

        InetAddress address;
        try {
            address = InetAddress.getByName(parts[0]);
        } catch (UnknownHostException e) {
            throw new IllegalArgumentException("Invalid CIDR string " + cidr);
        }

        int prefixLength = 0;
        if (parts.length == 1) {
            if (address instanceof Inet4Address) {
                prefixLength = 32;
            } else {
                prefixLength = 128;
            }
        } else {
            prefixLength = Integer.parseInt(parts[1]);
        }

        return new Pair<>(address, prefixLength);
    }

    /** Create a `VpnService.Builder` from the arguments passed in `intent`. */
    private Builder createVpnServiceBuilderFromStartIntent(Intent intent) {
        Builder builder = new Builder();

        // IP address.
        boolean hasIpv4 = false;
        boolean hasIpv6 = false;
        String overlayAddresses = intent.getStringExtra(OVERLAY_ADDRESSES_KEY);
        if (overlayAddresses == null) {
            overlayAddresses = DEFAULT_OVERLAY_ADDRESS;
        }
        String[] addresses = overlayAddresses.split(",");
        for (String address : addresses) {
            Pair<InetAddress, Integer> addressWithPrefix = parseIpCidrString(address);
            builder.addAddress(addressWithPrefix.first, addressWithPrefix.second);
            hasIpv4 |= addressWithPrefix.first instanceof Inet4Address;
            hasIpv6 |= addressWithPrefix.first instanceof Inet6Address;
        }

        // Routes.
        String includedRoutes = intent.getStringExtra(INCLUDED_ROUTES_KEY);
        if (includedRoutes != null) {
            String[] routes = includedRoutes.split(",");
            for (String route : routes) {
                Pair<InetAddress, Integer> prefix = parseIpCidrString(route);
                builder.addRoute(prefix.first, prefix.second);
            }
        } else {
            // If there is no included route set, install default routes for the enabled IP family.
            if (hasIpv4) {
                builder.addRoute("0.0.0.0", 0);
            }
            if (hasIpv6) {
                builder.addRoute("::", 0);
            }
        }

        String excludedRoutes = intent.getStringExtra(EXCLUDED_ROUTES_KEY);
        if (excludedRoutes != null) {
            if (VERSION.SDK_INT >= VERSION_CODES.TIRAMISU) {
                String[] routes = excludedRoutes.split(",");
                for (String route : routes) {
                    Pair<InetAddress, Integer> prefix = parseIpCidrString(route);
                    builder.excludeRoute(new IpPrefix(prefix.first, prefix.second));
                }
            } else {
                Log.w(TAG, EXCLUDED_ROUTES_KEY + " specified but not supported");
            }
        }

        // DNS.
        String dnsServer = intent.getStringExtra(DNS_SERVER_KEY);
        if (dnsServer != null) {
            builder.addDnsServer(dnsServer);
        } else {
            builder.addDnsServer(DEFAULT_DNS_SERVER);
        }

        // MTU.
        builder.setMtu(intent.getIntExtra(MTU_KEY, DEFAULT_MTU));

        return builder;
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        Log.d(TAG, "onStartCommand is called");
        showNotification();

        // Registers ourselves as an actual VpnService and sets up the underlying interface.
        VpnService.prepare(getApplicationContext());
        mTunFd = createVpnServiceBuilderFromStartIntent(intent)
                // Make sure read on the returned tun fd will be blocked, so that our programming
                // model will be easier.
                .setBlocking(true)
                .establish();

        // Connect to VPN server if arguments are given.
        String ifname = intent.getStringExtra(INTERFACE_KEY);
        String serverAddress = intent.getStringExtra(ADDRESS_KEY);
        int serverPort = intent.getIntExtra(PORT_KEY, INVALID_PORT);
        if (ifname != null && serverAddress != null && serverPort != INVALID_PORT) {
            connectToToyVpnServer(
                    ifname, serverAddress, serverPort, intent.getIntExtra(MTU_KEY, DEFAULT_MTU));
        } else {
            Log.d(TAG, "Arguments for connecting to toy VPN server is invalid, ifname: " + ifname +
                    ", server address: " + serverAddress + ", server port: " + serverPort +
                    ", ignore if this is expected");
        }

        // Setup socket if arguments are given.
        String sockProto = intent.getStringExtra(SOCKET_PROTOCOL_KEY);
        String sockIfname = intent.getStringExtra(SOCKET_INTERFACE_KEY);
        String sockAddress = intent.getStringExtra(SOCKET_ADDRESS_KEY);
        int sockPort = intent.getIntExtra(SOCKET_PORT_KEY, INVALID_PORT);
        if (sockProto != null && sockIfname != null && sockAddress != null
                && sockPort != INVALID_PORT) {
            setupSocket(sockProto, sockIfname, sockAddress, sockPort);
        } else {
            Log.d(TAG, "Arguments for setting up socket is invalid, proto: " + sockProto +
                    ", ifname: " + sockIfname + ", address: " + sockAddress + ", port: " +
                    sockPort + ", ignore if this is expected");
        }
        mBroadcastReceiver = new ArcVpnBroadcastReceiver();
        IntentFilter intentFilter = new IntentFilter();
        intentFilter.addAction(SEND_MESSAGE);
        registerReceiver(mBroadcastReceiver, intentFilter);
        return START_NOT_STICKY;
    }

    /** Setup socket by connecting to address:port with proto via ifname. */
    private void setupSocket(String proto, String ifname, String address, int port) {
        Log.d(TAG, "Start setting up socket, proto: " + proto + ", ifname: " + ifname +
                ", address: "+ address + ", port: " + port);
        InetAddress inetAddress;
        try {
            inetAddress = InetAddress.getByName(address);
        } catch (UnknownHostException e) {
            Log.e(TAG, "Address is unknown, setup socket failed, message:", e);
            return;
        }
        switch (proto) {
            case PROTOCOL_TCP:
                setupTcpSocket(ifname, inetAddress, port);
                break;
            case PROTOCOL_UDP:
                setupUdpSocket(ifname, inetAddress, port);
                break;
            default:
                Log.e(TAG, "Invalid procotol: " + proto + ", setup socket failed.");
        }
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

    /**
     * Connects to the toy VPN server listening at `address`:`port` via interface `ifname`. Also
     * starts the packet forwarding between the TCP connection tun interface after that.
     */
    private void connectToToyVpnServer(String ifname, String address, int port, int mtu) {
        Log.d(TAG, "Start connecting to toy VPN server, ifname: " + ifname + ", address: "+
                address + ", port: " + port + ", mtu: " + mtu);
        InetAddress inetAddress;
        try {
            inetAddress = InetAddress.getByName(address);
        } catch (UnknownHostException e) {
            Log.e(TAG, "Address is unknown, setup socket failed", e);
            return;
        }
        setupTcpSocket(ifname, inetAddress, port);
        startForwarding(mtu);
    }

    /**
     * Sets up a TCP socket using given address and port of the remote peer we want to connect to,
     * and the name of the interface in ARC that we want to use to setup the socket.
     * When called multiple times in one test, the older socket will be replaced by newly setup
     * socket for sending messages.
     */
    private void setupTcpSocket(String ifname, InetAddress address, int port) {
        mExecutor.submit(() -> {
            Log.d(TAG, "Start setting up TCP socket, ifname: "+ifname +
            ", address: "+ address.toString() + ", port: " + port);
            try {
                Network net = getNetworkByInterface(ifname);
                if (net == null) {
                    Log.e(TAG, "Network with specified interface name does not exist, set up "
                            + "socket failed.");
                    return;
                }
                mTcpSocket = net.getSocketFactory().createSocket();
                protect(mTcpSocket);
                mTcpSocket.connect(new InetSocketAddress(address, port));
                mWriter = new PrintWriter(mTcpSocket.getOutputStream(), /*autoFlush=*/ true);
                mLastSetupSocketFamily = PROTOCOL_TCP;
                Log.d(TAG, "Setting up TCP socket succeed");
            } catch (IOException e) {
                Log.e(TAG, "Error opening TCP socket", e);
            }
        });
    }

    /**
     * Sends a message via setup TCP socket. This method needs to be called after calling
     * {@link #setupTcpSocket()}.
     */
    public void sendTcpMessage(String msg) {
        mExecutor.submit(() -> {
            Log.d(TAG, "sendTcpMessage");
            try {
                if (mWriter == null) {
                    Log.e(TAG, "TCP socket has not been set up yet, send message failed.");
                    return;
                }
                mWriter.println(msg);
                Log.d(TAG, "sendTcpMessage succeed");
            } catch (Exception e) {
                Log.e(TAG, "Failed to send TCP messages", e);
            }
        });
    }

    /**
     * Sets up a UDP socket using given address and port of the remote peer we want to connect to,
     * and the name of the interface in ARC that we want to use to setup the socket.
     * When called multiple times in one test, the older socket will be replaced by newly setup
     * socket for sending messages.
     */
    private void setupUdpSocket(String ifname, InetAddress address, int port) {
        mExecutor.submit(() -> {
            Log.d(TAG, "Start setting up UDP socket, ifname: "+ifname+
            ", address: "+ address.toString() + ", port: " + port);
            try {
                Network net = getNetworkByInterface(ifname);
                if (net == null) {
                    Log.e(TAG, "Network with specified interface name does not exist, set up "
                            + "socket failed.");
                    return;
                }
                mUdpSocket = new DatagramSocket();
                net.bindSocket(mUdpSocket);
                protect(mUdpSocket);
                mUdpSocket.connect(address, port);
                mLastSetupSocketFamily = PROTOCOL_UDP;
                Log.d(TAG, "Setting up UDP socket succeed");
            } catch (IOException e) {
                Log.e(TAG, "Error opening UDP socket", e);
            }
        });
    }

    /**
     * Sends a message via setup UDP socket. This method needs to be called after calling
     * {@link #setupUdpSocket()}.
     */
    public void sendUdpMessage(String msg) {
        mExecutor.submit(() -> {
            try {
                Log.d(TAG, "sendUdpMessage");
                if (mUdpSocket == null) {
                    Log.e(TAG, "UDP socket has not been set up yet, send message failed.");
                    return;
                }
                DatagramPacket dp = new DatagramPacket(msg.getBytes(), msg.length(),
                        mUdpSocket.getRemoteSocketAddress());
                mUdpSocket.send(dp);
                Log.d(TAG, "sendUdpMessage succeed");
            } catch (IOException e) {
                Log.e(TAG, "Failed to send UDP messages", e);
            }
        });
    }

    /**
     * A helper function to read input into `b`. Different from the `read()` on InputStream, this
     * function will try to read exactly len bytes before return. Returns -1 if EOF is reached
     * before `len` bytes are read.
     */
    static private int readExact(InputStream input, byte[] b, int len) throws IOException {
        int readTotal = 0;
        while (readTotal < len) {
            int cnt = input.read(b, readTotal, len - readTotal);
            if (cnt == -1) {
                return -1;
            }
            readTotal += cnt;
        }
        return readTotal;
    }

    /**
     * Starts two threads to do the bidirectional forwarding between TUN device and TCP socket.
     */
    private void startForwarding(int mtu) {
        // Wrap this as a task and post it in the executor because the TCP socket is set up
        // asynchronously.
        mExecutor.submit(() -> {
            if (mTcpSocket == null) {
                Log.e(TAG, "TCP connection to the server has not been established");
                return;
            }
            if (mTunFd == null) {
                Log.e(TAG, "TUN device is not ready");
                return;
            }

            // TCP -> TUN. Read a message from TCP connection which contains a packet length and an
            // IP packet, and write the IP packet to the TUN device. Note that for a TCP socket, it
            // cannot be guaranteed that one read can get the whole message or packet, and thus we
            // need to do a loop to read until we get enough bytes.
            new Thread(()-> {
                try {
                    byte[] headerBytes = new byte[TOY_VPN_MESSAGE_HEADER_SIZE];
                    InputStream input = mTcpSocket.getInputStream();
                    try (OutputStream output = new FileOutputStream(mTunFd.getFileDescriptor())) {
                        while (true) {
                            // Read the header as length.
                            int readCnt = readExact(
                                    input, headerBytes, TOY_VPN_MESSAGE_HEADER_SIZE);
                            if (readCnt != TOY_VPN_MESSAGE_HEADER_SIZE) {
                                Log.i(TAG, "Read header bytes returned " + readCnt
                                        + ", assume connection ended");
                                break;
                            }
                            int length = ByteBuffer.wrap(headerBytes).getInt();

                            // Read the payload as packet.
                            byte[] payloadBytes = new byte[length];
                            readCnt = readExact(input, payloadBytes, length);
                            if (readCnt != length) {
                                Log.e(TAG, "Failed to read the payload, got " + readCnt
                                        + ", want " + length);
                                break;
                            }

                            // Write the packet to tun interface.
                            output.write(payloadBytes);
                        }
                    }
                } catch (IOException e) {
                    Log.e(TAG, "Failed to forward from TCP connection to TUN device", e);
                }
            }).start();

            // TUN -> TCP. Read an IP packet from the TUN device, compose a message which is the
            // length of this packet and the IP packet itself, and write it to the TCP connection.
            new Thread(()-> {
                try {
                    byte[] payloadBytes = new byte[mtu * 2];
                    OutputStream output = mTcpSocket.getOutputStream();
                    try (InputStream input = new FileInputStream(mTunFd.getFileDescriptor())) {
                        while (true) {
                            // Read the packet.
                            int readCnt = input.read(payloadBytes);
                            if (readCnt == -1) {
                                Log.i(TAG, "Read returned -1, assume connection ended");
                                break;
                            }

                            // Write the length as header.
                            byte[] headerBytes = ByteBuffer.allocate(TOY_VPN_MESSAGE_HEADER_SIZE)
                                                           .putInt(readCnt)
                                                           .array();
                            output.write(headerBytes);

                            // Write the packet as payload.
                            output.write(payloadBytes, /*off=*/0, readCnt);
                        }
                    }
                } catch (IOException e) {
                    Log.e(TAG, "Failed to forward from TUN device to TCP connection", e);
                }
            }).start();
        });
    }

    /**
     * Gets the network whose interface matches the specified interface from all available ARC
     * networks. Note that the interface name is the interface name within ARC, not in host.
     *
     * @return the network that matches the interface, null if it doesn't exist
     */
    private Network getNetworkByInterface(String ifname) {
        ConnectivityManager connectivityManager = (ConnectivityManager) getApplicationContext()
                .getSystemService(Context.CONNECTIVITY_SERVICE);
        if (connectivityManager == null) {
            Log.e(TAG, "Get connected WiFi network failed, failed to get ConnectivityManager.");
            return null;
        }
        Network[] networks = connectivityManager.getAllNetworks();
        for (Network network : networks) {
            LinkProperties linkProperties = connectivityManager.getLinkProperties(network);
            if (linkProperties.getInterfaceName().equals(ifname)) {
                return network;
            }
        }
        return null;
    }
}
