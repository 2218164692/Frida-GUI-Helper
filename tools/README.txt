Frida GUI Helper local tools layout
===================================

The app resolves tools in this directory before falling back to the system PATH.

Recommended Windows layout:

tools/
  platform-tools/
    adb.exe
    AdbWinApi.dll
    AdbWinUsbApi.dll

  frida/
    frida.exe
    frida-ps.exe
    frida-ls-devices.exe

  frida-server/
    android-arm64/
      frida-server
    android-arm/
      frida-server
    android-x86/
      frida-server
    android-x86_64/
      frida-server

The Android frida-server version must match the local Frida CLI version exactly.
