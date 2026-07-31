Unicode true

####
## Frida GUI Helper installer, based on the Wails v2.12 NSIS template.
##
## Wails generates wails_tools.nsh and the WebView2 bootstrapper at build time.
## The tools/build-installer.ps1 script stages the bundled runtime under
## payload/frida before invoking `wails build -nsis`.
####

# Install for the current user so the managed Frida component can be updated
# later without requiring administrator rights.
!define REQUEST_EXECUTION_LEVEL "user"
!include "wails_tools.nsh"

# The version information for these fields must consist of four parts.
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING
!define INSTALL_MARKER ".frida-gui-helper-owned"

!insertmacro MUI_PAGE_WELCOME
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE ValidateInstallDirectory
!insertmacro MUI_PAGE_DIRECTORY
!undef MUI_PAGE_CUSTOMFUNCTION_LEAVE
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\Frida-GUI-Helper-v${INFO_PRODUCTVERSION}-windows-${ARCH}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
# Reuse the directory chosen for an earlier installation when upgrading.
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
ShowInstDetails show

Function .onInit
    !insertmacro wails.checkArchitecture
FunctionEnd

Function ValidateInstallDirectory
    # A selected folder may contain unrelated user data. Refuse to replace an
    # existing frida directory unless this installer created it previously.
    IfFileExists "$INSTDIR\frida\*.*" 0 directory_is_safe
    IfFileExists "$INSTDIR\${INSTALL_MARKER}" directory_is_safe 0
    MessageBox MB_ICONEXCLAMATION|MB_OK "The selected folder already contains a Frida directory that is not owned by Frida GUI Helper. Please choose another installation folder."
    Abort

directory_is_safe:
FunctionEnd

Section
    !insertmacro wails.setShellContext

    # Repeat the ownership check here because page callbacks are skipped by
    # silent installations.
    IfFileExists "$INSTDIR\frida\*.*" 0 runtime_is_safe
    IfFileExists "$INSTDIR\${INSTALL_MARKER}" runtime_is_safe 0
    SetErrorLevel 2
    Abort "The selected folder contains an unmanaged Frida directory."

runtime_is_safe:

    !insertmacro wails.webview2runtime

    SetOutPath "$INSTDIR"
    !insertmacro wails.files

    # Install the self-contained desktop Frida CLI and Android server binaries.
    # The application resolves tools from this directory and does not use PATH.
    RMDir /r "$INSTDIR\frida"
    SetOutPath "$INSTDIR\frida"
    File /r "payload\frida\*.*"
    SetOutPath "$INSTDIR"
    FileOpen $0 "$INSTDIR\${INSTALL_MARKER}" w
    FileWrite $0 "Frida GUI Helper ${INFO_PRODUCTVERSION}$\r$\n"
    FileClose $0

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

	WriteUninstaller "$INSTDIR\uninstall.exe"
    SetRegView 64
    WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${INFO_COMPANYNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${INFO_PRODUCTNAME}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${INFO_PRODUCTVERSION}"
    WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
    WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
    WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"

    ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
    IntFmt $0 "0x%08X" $0
    WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # Never recursively remove $INSTDIR. It may contain user-created files or
    # may have been deliberately shared with another application.
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    IfFileExists "$INSTDIR\${INSTALL_MARKER}" 0 skip_owned_runtime
    RMDir /r "$INSTDIR\frida"
skip_owned_runtime:
    Delete "$INSTDIR\${INSTALL_MARKER}"

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    Delete "$INSTDIR\uninstall.exe"
    SetRegView 64
    DeleteRegKey HKCU "${UNINST_KEY}"
    # This succeeds only when the selected directory contains no user files.
    RMDir "$INSTDIR"
SectionEnd
