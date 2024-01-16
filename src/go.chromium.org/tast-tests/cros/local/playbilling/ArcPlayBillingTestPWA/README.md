# README: Arc Play Billing Test PWA

## Introduction

This directory contains the source code for creating the Arc Play Billing Test
PWA using [bubblewrap].

## Keystore passwords

Keystore passwords are kept in http://valentine under ArcPlayBillingTestPWA.

## Bubblewrap Build

1. Install bubblewrap
```
sudo npm i -g @bubblewrap/cli
```

2. Start a web server in the ArcPlayBillingTestPWA dir.
```
python -m http.server 8080
```

3. bubblewrap build
```
bubblewrap build
```

4. Edit `app/src/main/AndroidManifest.xml` and change scope `https` to be
   `http`.

5. Rebuild bubblewrap
```
bubblewrap build
```

6. The resulting `app-release-signed.apk` is the APK to upload to play and use
   in tests.

[bubblewrap]: https://github.com/GoogleChromeLabs/bubblewrap
