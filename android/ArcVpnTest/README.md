# ArcVpnTest app

This Android app installs, preauthorizes and starts a VpnService to register a VPN with the Android
networking stack, setup sockets and send messages out of the tun interface.

Before the tunnel can be created, the app must be installed and preauthorized, which is already provided by this package. Afterwards, starting the service should work.

To start the service, run

`adb shell am broadcast -a org.chromium.arc.testapp.arcvpn.LAUNCH_VPN --receiver-include-background`

Alternatively, if the app is supposed to connect to [the toy VPN server](https://crrev.com/c/5647430) for tunneling the traffic, run

```
adb shell am broadcast -a org.chromium.arc.testapp.arcvpn.LAUNCH_VPN \
        --receiver-include-background \
        --es interface <ARC_IFNAME> \
        --es address <SERVER_IP> --ei port <SERVER_TCP_PORT> \
        --es overlay_address <LOCAL_OVERLAY_IPV4> \
        --es dns_server <DNS_SERVER>
```

if a socket is required to be set up when service is started, run:

```
adb shell am broadcast -a org.chromium.arc.testapp.arcvpn.LAUNCH_VPN \
        --receiver-include-background \
        --es sockinterface <ARC_IFNAME> \
        --es sockaddress <DST_ADDR> --ei sockport <DST_PORT> \
        --es sockproto <PROTOCOL>
```

The socket is set up with protocol(either `tcp` or `udp`), remote peer address and port, and interface name interface name in ARC that we want to setup socket with.

After setting up socket, we can send messages from the last setup socket by command:

`adb shell am broadcast -a org.chromium.arc.testapp.arcvpn.SEND_MESSAGE --es message <MESSAGE>`
