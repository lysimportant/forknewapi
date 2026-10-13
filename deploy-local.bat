@echo off
setlocal EnableExtensions DisableDelayedExpansion
cd /d "%~dp0"
if errorlevel 1 exit /b 1

rem One-click deployment settings. The password stays in the existing ssh.txt file.
set "SSH_HOST=47.242.207.224"
set "SSH_USER=root"
set "SSH_PORT=22"
set "REMOTE_DIR=/opt/forknewapi"
set "CREDENTIAL_FILE=%USERPROFILE%\Desktop\ssh.txt"

set "DEPLOY_DEPS=%~dp0.local-tests\deploy-deps"
if not exist "%CREDENTIAL_FILE%" goto :missing_credentials
where python >nul 2>&1
if errorlevel 1 goto :missing_python

:check_dependency
python -c "import sys; sys.path.insert(0, r'%DEPLOY_DEPS%'); import paramiko" >nul 2>&1
if not errorlevel 1 goto :run_deploy
echo Installing the SSH helper into .local-tests...
python -m pip install --isolated --no-cache-dir --index-url https://pypi.org/simple --timeout 10 --retries 2 --disable-pip-version-check --target "%DEPLOY_DEPS%" paramiko==4.0.0
if errorlevel 1 goto :setup_failed

:run_deploy
python "%~dp0deployment\deploy-local.py" --host "%SSH_HOST%" --user "%SSH_USER%" --port "%SSH_PORT%" --remote-dir "%REMOTE_DIR%" --credential-file "%CREDENTIAL_FILE%" %*
set "RESULT=%ERRORLEVEL%"
if not defined DEPLOY_NO_PAUSE pause
exit /b %RESULT%

:missing_credentials
echo Credential file not found: %CREDENTIAL_FILE%
goto :failed
:missing_python
echo Python 3 is required to run this deployment script.
goto :failed
:setup_failed
echo Could not prepare the local deployment helper.
:failed
if not defined DEPLOY_NO_PAUSE pause
exit /b 1
