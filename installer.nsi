; ============================================
; TimeKeeper Installer
; ============================================

!include "MUI2.nsh"

!define APP_NAME "TimeKeeper"
!define APP_VERSION "1.0.0"
!define APP_EXE "TimeKeeper.exe"
!define INSTALL_FOLDER "TimeKeeper"

; ============================================
; GENERAL
; ============================================

Name "${APP_NAME}"
OutFile "TimeKeeperSetup.exe"

InstallDir "$PROGRAMFILES\${INSTALL_FOLDER}"

RequestExecutionLevel admin

; ============================================
; MUI SETTINGS
; ============================================

!define MUI_ABORTWARNING

; Hiển thị Welcome page
!insertmacro MUI_PAGE_WELCOME

; Chọn thư mục cài đặt
!insertmacro MUI_PAGE_DIRECTORY

; Quá trình cài đặt
!insertmacro MUI_PAGE_INSTFILES

; Finish page
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Run TimeKeeper"
!define MUI_FINISHPAGE_RUN_CHECKED

!insertmacro MUI_PAGE_FINISH

; ============================================
; LANGUAGE
; ============================================

!insertmacro MUI_LANGUAGE "English"

; ============================================
; INSTALL
; ============================================

Section "Install"

    ; Tạo thư mục cài đặt
    SetOutPath "$INSTDIR"

    ; Copy TimeKeeper.exe
    File "${APP_EXE}"

    ; Tạo Uninstaller
    WriteUninstaller "$INSTDIR\Uninstall.exe"

SectionEnd

; ============================================
; UNINSTALL
; ============================================

Section "Uninstall"

    ; Dừng Windows Service trước khi xóa file để tránh bị khóa file (file lock)
    ExecWait '"$INSTDIR\${APP_EXE}" stop'

    ; Gỡ đăng ký Windows Service khỏi Service Control Manager
    ExecWait '"$INSTDIR\${APP_EXE}" uninstall'

    ; Xóa TimeKeeper.exe
    Delete "$INSTDIR\${APP_EXE}"

    ; Xóa Uninstaller
    Delete "$INSTDIR\Uninstall.exe"

    ; Xóa thư mục
    RMDir "$INSTDIR"

SectionEnd