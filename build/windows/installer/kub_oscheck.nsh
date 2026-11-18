; Replaces wails.checkArchitecture (wails_tools.nsh is regenerated on every
; build, so the check lives here).
;
; Wails' check uses ${AtLeastWin10} from WinVer.nsh, which trusts the version
; the process is told. A compatibility layer (e.g. "~ WIN7RTM" set on
; kub-connect.exe) is inherited by the installer the in-app updater launches,
; so on Windows 11 it saw "Windows 7" and refused to install. The registry
; value CurrentMajorVersionNumber is not shimmed and exists only on Win10+.
;
; The Windows 7 artifact is built with KUB_WIN7=1 in the environment and
; skips the version check (it runs on 7 and newer).

!include "LogicLib.nsh"
!include "x64.nsh"

!macro kub.checkWindows
    ${IfNot} ${IsNativeAMD64}
        IfSilent +2
            MessageBox MB_OK|MB_ICONSTOP "KUB Connect поддерживает только 64-битную Windows (x64)."
        SetErrorLevel 65
        Quit
    ${EndIf}

    !if "$%KUB_WIN7%" != "1"
        ClearErrors
        ReadRegDWORD $0 HKLM "SOFTWARE\Microsoft\Windows NT\CurrentVersion" "CurrentMajorVersionNumber"
        ${If} ${Errors}
        ${OrIf} $0 < 10
            IfSilent +2
                MessageBox MB_OK|MB_ICONSTOP "Этот установщик для Windows 10 и новее.$\nДля Windows 7/8 скачайте установщик с пометкой windows7."
            SetErrorLevel 64
            Quit
        ${EndIf}
    !endif
!macroend
