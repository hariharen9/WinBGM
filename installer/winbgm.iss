; WinBGM Inno Setup Script
; Parameterized via /D switches from CI:
;   /DAppVersion=1.1.0
;   /DArch=x64  (x64 | arm64 | x86)
;   /DSourceExe=build\bin\WinBGM.exe
;   /DOutputDir=.
;   /DOutputFilename=WinBGM-Setup-v1.1.0-windows-x64

#ifndef AppVersion
  #define AppVersion "1.1.0"
#endif
#ifndef Arch
  #define Arch "x64"
#endif
#ifndef SourceExe
  #define SourceExe "build\bin\WinBGM.exe"
#endif
#ifndef OutputDir
  #define OutputDir "."
#endif
#ifndef OutputFilename
  #define OutputFilename "WinBGM-Setup-v" + AppVersion + "-windows-" + Arch
#endif

[Setup]
AppId={{6D2F1C3A-4B8E-4F2A-9C1D-7E3B5A8F0D2C}
AppName=WinBGM
AppVersion={#AppVersion}
AppVerName=WinBGM {#AppVersion}
AppPublisher=Hariharen
AppPublisherURL=https://hariharen.space
AppSupportURL=https://github.com/hariharen9/WinBGM/issues
AppUpdatesURL=https://github.com/hariharen9/WinBGM/releases
DefaultDirName={autopf}\WinBGM
DefaultGroupName=WinBGM
AllowNoIcons=yes
; Run after install
InfoAfterFile=
; Uninstaller icon comes from the installed exe
UninstallDisplayIcon={app}\WinBGM.exe
UninstallDisplayName=WinBGM
; Output
OutputDir={#OutputDir}
OutputBaseFilename={#OutputFilename}
; Compression
Compression=lzma2/max
SolidCompression=yes
; Visuals
WizardStyle=modern
SetupIconFile=build\windows\icon.ico
; Require admin for Program Files installation
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
; Architecture
#if Arch == "x64"
  ArchitecturesAllowed=x64compatible
  ArchitecturesInstallIn64BitMode=x64compatible
#elif Arch == "arm64"
  ArchitecturesAllowed=arm64
  ArchitecturesInstallIn64BitMode=arm64
#endif
; Minimum Windows version: Windows 10
MinVersion=10.0.17763

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "WinBGM.exe"; Flags: ignoreversion

[Icons]
Name: "{group}\WinBGM";         Filename: "{app}\WinBGM.exe"
Name: "{group}\Uninstall WinBGM"; Filename: "{uninstallexe}"
Name: "{autodesktop}\WinBGM";   Filename: "{app}\WinBGM.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\WinBGM.exe"; Description: "{cm:LaunchProgram,WinBGM}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "{app}\WinBGM.exe"; Parameters: "--quit"; Flags: skipifdoesntexist runhidden
