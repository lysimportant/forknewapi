@echo off
setlocal EnableExtensions DisableDelayedExpansion
cd /d "%~dp0"
if errorlevel 1 exit /b 1

rem Build a linux/amd64 image for the x86_64 server. This script never connects to it.
set "IMAGE=forknewapi:local"
set "PLATFORM=linux/amd64"
set "OUTPUT_DIR=%~dp0upload"
set "OUTPUT_TAR=%OUTPUT_DIR%\forknewapi-local.tar"

where docker >nul 2>&1
if errorlevel 1 goto :docker_missing
docker info >nul 2>&1
if errorlevel 1 goto :docker_unavailable

echo Building %IMAGE% for %PLATFORM%...
docker build --platform "%PLATFORM%" --tag "%IMAGE%" .
if errorlevel 1 goto :build_failed

docker image inspect "%IMAGE%" >nul 2>&1
if errorlevel 1 goto :image_missing

if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"
if errorlevel 1 goto :output_failed

if exist "%OUTPUT_TAR%" del /f /q "%OUTPUT_TAR%"
if exist "%OUTPUT_TAR%" goto :output_failed

echo Saving Docker image to:
echo %OUTPUT_TAR%
docker save --output "%OUTPUT_TAR%" "%IMAGE%"
if errorlevel 1 goto :save_failed
if not exist "%OUTPUT_TAR%" goto :save_failed

for %%A in ("%OUTPUT_TAR%") do set "OUTPUT_SIZE=%%~zA"
echo.
echo Package ready.
echo File: %OUTPUT_TAR%
echo Size: %OUTPUT_SIZE% bytes
echo.
echo Upload this .tar file to /opt/forknewapi/ on the server,
echo then run the commands provided in the chat.
if not defined PACKAGE_NO_PAUSE pause
exit /b 0

:docker_missing
echo Docker was not found in PATH.
goto :failed
:docker_unavailable
echo Docker is not running. Start Docker Desktop and try again.
goto :failed
:output_failed
echo Could not prepare the output directory or remove the old package.
goto :failed
:build_failed
echo Docker image build failed.
goto :failed
:image_missing
echo The built Docker image could not be inspected.
goto :failed
:save_failed
echo Docker image export failed.
:failed
if not defined PACKAGE_NO_PAUSE pause
exit /b 1
