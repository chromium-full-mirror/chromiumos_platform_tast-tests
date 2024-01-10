# ArcVpnTest app

This Android app starts a VpnService to register a VPN with the Android
networking stack. It does not actually forward data in/out of the tun interface.

Before the tunnel can be created, the app must be allowed to start the
VpnService. This is usually done through a UI popup that the user checks. The
app can also be pre-authorized with:

`adb shell dumpsys wifi authorize-vpn org.chromium.arc.testapp.arcvpn`

Afterwards, starting the service should work. To start the service, run

`adb shell am broadcast -a org.chromium.arc.testapp.arcvpn.LAUNCH_VPN --receiver-include-background`
