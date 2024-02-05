# ArcVpnTest app

This Android app starts a VpnService to register a VPN with the Android
networking stack, setup sockets and send messages out of the tun interface.

Before the tunnel can be created, the app must be allowed to start the
VpnService. This is usually done through a UI popup that the user checks. The
app can also be pre-authorized with:

`adb shell dumpsys wifi authorize-vpn org.chromium.arc.testapp.arcvpn`

Afterwards, starting the service should work. To start the service, run

`adb shell am broadcast -a org.chromium.arc.testapp.arcvpn.LAUNCH_VPN --receiver-include-background`

We can setup socket by passing command with protocol(either `tcp` or `udp`), remote peer address and port with:

`adb shell am broadcast -a org.chromium.arc.testapp.arcvpn.SETUP_SOCKET --es proto <PROTO> --es address <ADDR> --ei port <PORT>`

After setting up socket, we can send messages from the last setup socket by command:

`adb shell am broadcast -a org.chromium.arc.testapp.arcvpn.SEND_MESSAGE --es message <MESSAGE>`
