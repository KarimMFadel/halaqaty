@echo off
rem Helper: run Flutter from Git Bash where PROGRAMFILES(X86) is missing.
set "PROGRAMFILES(X86)=C:\Program Files (x86)"
"C:\myPersonalData\programs\flutterSDK\flutter\bin\flutter.bat" %*
