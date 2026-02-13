@echo off
echo Building DbBridge...
go build -o dbbridge.exe ./cmd/dbbridge
if %errorlevel% neq 0 (
    echo Build Failed!
    exit /b %errorlevel%
)
echo Build Success! dbbridge.exe created.
echo.
echo To run: dbbridge.exe
pause
