@echo off
REM Build script for MPT-II Go thermal printer tools (Windows)

echo Building MPT-II Go thermal printer tools...

REM Create bin directory
if not exist bin mkdir bin

REM Build main CLI tool
echo Building mptprinter-cli...
go build -o bin/mptprinter-cli.exe ./cmd/mptprinter-cli
if %errorlevel% equ 0 (
    echo ✓ mptprinter-cli built successfully
) else (
    echo ✗ Failed to build mptprinter-cli
    exit /b 1
)

REM Build simple print tool
echo Building mptprint...
go build -o bin/mptprint.exe ./cmd/mptprint
if %errorlevel% equ 0 (
    echo ✓ mptprint built successfully
) else (
    echo ✗ Failed to build mptprint
    exit /b 1
)

echo.
echo Build complete! Tools available in .\bin\
echo.
echo Usage:
echo   .\bin\mptprint.exe "Hello, World!"                       # Simple printing
echo   .\bin\mptprinter-cli.exe -text "Hello" -bold -center     # Advanced printing
echo.
echo For help:
echo   .\bin\mptprinter-cli.exe -help
