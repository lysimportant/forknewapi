"""Build a local Docker image and deploy it to the configured Compose server."""

from __future__ import annotations

import argparse
import codecs
import hashlib
import re
import shlex
import subprocess
import sys
import tempfile
import time
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / ".local-tests" / "deploy-deps"))

import paramiko


IMAGE = "forknewapi:local"
COMPOSE_FILE = "docker-compose.yml"
APP_SERVICE = "new-api"
DEPENDENCIES = ("postgres", "redis")


class DeployError(Exception):
    pass


def read_password(path: Path, user: str, host: str) -> str:
    try:
        raw = path.read_bytes()
    except OSError as exc:
        raise DeployError("Cannot read the credential file.") from exc

    for encoding in ("utf-8-sig", "utf-16", "gb18030"):
        try:
            content = raw.decode(encoding)
            break
        except UnicodeDecodeError:
            continue
    else:
        raise DeployError("Cannot decode the credential file.")

    lines = [line for line in content.splitlines() if line.strip()]
    if len(lines) != 2:
        raise DeployError("The credential file must contain an account line and a password line.")

    account = lines[0].strip()
    target = re.search(r"([\w.-]+)@([\w.:-]+)", account)
    if target:
        if target.group(1) != user or target.group(2) != host:
            raise DeployError("The credential file targets a different SSH account or server.")
    elif not re.search(rf"\b{re.escape(user)}\b", account):
        raise DeployError("The credential file does not identify the configured SSH user.")

    password = lines[1]
    labeled = re.match(r"\s*(?:password|passwd|密码|口令)\s*[:=：]\s*(.*)", password, re.I)
    if labeled:
        password = labeled.group(1)
    if not password:
        raise DeployError("The credential file has an empty password line.")
    return password


def remote(client: paramiko.SSHClient, command: str, *, stream: bool = False, timeout: int = 300) -> str:
    transport = client.get_transport()
    if transport is None or not transport.is_active():
        raise DeployError("The SSH connection closed unexpectedly.")
    channel = transport.open_session(timeout=20)
    channel.set_combine_stderr(True)
    channel.exec_command(command)
    chunks: list[bytes] = []
    decoder = codecs.getincrementaldecoder("utf-8")("replace")
    deadline = time.monotonic() + timeout
    try:
        while not channel.exit_status_ready() or channel.recv_ready():
            if time.monotonic() > deadline:
                raise DeployError("A server command timed out.")
            if channel.recv_ready():
                chunk = channel.recv(65536)
                chunks.append(chunk)
                if stream:
                    print(decoder.decode(chunk), end="", flush=True)
            else:
                time.sleep(0.1)
        status = channel.recv_exit_status()
        output_raw = b"".join(chunks).decode("utf-8", "replace")
        output = output_raw.strip()
        if stream:
            print(decoder.decode(b"", final=True), end="", flush=True)
            if output_raw and not output_raw.endswith("\n"):
                print()
        if status != 0:
            if stream:
                raise DeployError(f"Server command failed with exit code {status}.")
            raise DeployError(f"Server check failed: {output[-800:] or f'exit code {status}'}")
        return output
    finally:
        channel.close()


def compose_prefix(remote_dir: str) -> str:
    return f"cd {shlex.quote(remote_dir)} && docker compose -f {shlex.quote(COMPOSE_FILE)}"


def preflight(client: paramiko.SSHClient, remote_dir: str) -> str:
    directory = shlex.quote(remote_dir)
    prefix = compose_prefix(remote_dir)
    remote(client, f"cd {directory} && test -f {COMPOSE_FILE} && test -f .env && docker info >/dev/null && command -v sha256sum >/dev/null")
    remote(client, f"{prefix} config --quiet")
    services = set(remote(client, f"{prefix} config --services").splitlines())
    needed = {APP_SERVICE, *DEPENDENCIES}
    if not needed.issubset(services):
        raise DeployError("The server Compose file does not contain the expected app, PostgreSQL, and Redis services.")
    images = set(remote(client, f"{prefix} config --images").splitlines())
    if IMAGE not in images:
        raise DeployError(f"The server Compose file must use image {IMAGE} for this deployment method.")
    compose_help = remote(client, "docker compose up --help")
    if not all(option in compose_help for option in ("--wait", "--wait-timeout", "--no-recreate", "--no-deps")):
        raise DeployError("The server Docker Compose version is too old for this deployment script.")
    arch = remote(client, "uname -m")
    platform = {"x86_64": "linux/amd64", "amd64": "linux/amd64", "aarch64": "linux/arm64", "arm64": "linux/arm64"}.get(arch)
    if platform is None:
        raise DeployError(f"Unsupported server architecture: {arch}")
    running = remote(client, f"{prefix} ps --services --status running")
    compose_version = remote(client, "docker compose version --short")
    print(f"Server: {arch}; Docker Compose {compose_version}; running: {', '.join(running.splitlines()) or 'none'}")
    return platform


def local_docker_check() -> None:
    result = subprocess.run(["docker", "info", "--format", "{{.OSType}}"], cwd=ROOT, capture_output=True, text=True)
    if result.returncode != 0:
        raise DeployError("Local Docker is unavailable. Start Docker Desktop and try again.")
    if result.stdout.strip() != "linux":
        raise DeployError("Docker Desktop must be running Linux containers.")


def run_local(arguments: list[str]) -> None:
    result = subprocess.run(arguments, cwd=ROOT)
    if result.returncode != 0:
        raise DeployError(f"Local command failed with exit code {result.returncode}: {arguments[0]} {arguments[1]}")


def connect_client(client: paramiko.SSHClient, host: str, port: int, user: str, password: str) -> None:
    client.connect(
        hostname=host,
        port=port,
        username=user,
        password=password,
        look_for_keys=False,
        allow_agent=False,
        timeout=20,
        auth_timeout=20,
        banner_timeout=20,
    )
    transport = client.get_transport()
    if transport is not None:
        transport.set_keepalive(30)


def deploy(
    client: paramiko.SSHClient,
    platform: str,
    remote_dir: str,
    *,
    host: str,
    port: int,
    user: str,
    password: str,
) -> None:
    prefix = compose_prefix(remote_dir)
    old_id = ""
    try:
        old_id = remote(client, f"docker image inspect --format '{{{{.Id}}}}' {shlex.quote(IMAGE)}")
    except DeployError:
        pass  # First deployment: there is no prior image to restore.
    if old_id and not re.fullmatch(r"sha256:[a-f0-9]{64}", old_id):
        raise DeployError("The server returned an invalid previous image ID.")

    with tempfile.TemporaryDirectory(prefix="new-api-deploy-") as temp_dir:
        archive = Path(temp_dir) / f"new-api-image-{uuid.uuid4().hex}.tar"
        print(f"Building {IMAGE} for {platform}...", flush=True)
        run_local(["docker", "build", "--platform", platform, "--tag", IMAGE, "."])
        print("Saving the Docker image...", flush=True)
        run_local(["docker", "save", "--output", str(archive), IMAGE])

        remote_archive = f"/tmp/{archive.name}"
        size = archive.stat().st_size
        free_kib = remote(client, "df -Pk /tmp | tail -n 1 | awk '{print $4}'")
        if not free_kib.isdecimal() or int(free_kib) * 1024 < size + 128 * 1024 * 1024:
            raise DeployError("The server does not have enough free space in /tmp for this image archive.")
        print(f"Uploading {size / 1024 / 1024:.1f} MiB to the server...", flush=True)
        try:
            upload_with_resume(
                client,
                archive,
                remote_archive,
                size,
                password=password,
                host=host,
                port=port,
                user=user,
            )
            print("Loading the image on the server...", flush=True)
            remote(client, f"docker load -i {shlex.quote(remote_archive)}", stream=True, timeout=600)
        finally:
            try:
                transport = client.get_transport()
                if transport is None or not transport.is_active():
                    connect_client(client, host, port, user, password)
                remote(client, f"rm -f {shlex.quote(remote_archive)}")
            except (DeployError, EOFError, OSError, paramiko.SSHException):
                print(f"Could not remove temporary server file: {remote_archive}", file=sys.stderr)

    print("Starting PostgreSQL and Redis if needed...", flush=True)
    remote(client, f"{prefix} up -d --no-build --no-recreate --wait --wait-timeout 240 {' '.join(DEPENDENCIES)}", stream=True, timeout=300)
    app_command = f"{prefix} up -d --no-build --no-deps --force-recreate --wait --wait-timeout 240 {APP_SERVICE}"
    print("Starting the new application image and waiting for its health check...", flush=True)
    try:
        remote(client, app_command, stream=True, timeout=300)
    except DeployError as startup_error:
        if not old_id:
            raise DeployError("The new application failed its health check; no previous image exists.") from startup_error
        print("The new application failed; restoring the previous image...", flush=True)
        remote(client, f"docker tag {shlex.quote(old_id)} {shlex.quote(IMAGE)}", stream=True)
        try:
            remote(client, app_command, stream=True, timeout=300)
        except DeployError as rollback_error:
            raise DeployError("The new application and the attempted restoration both failed. Check the server logs.") from rollback_error
        raise DeployError("The new application failed; the previous image was restored.") from startup_error
    print("Deployment completed and the application is healthy.")


def upload_with_resume(
    client: paramiko.SSHClient,
    local_path: Path,
    remote_path: str,
    size: int,
    *,
    password: str,
    host: str,
    port: int,
    user: str,
) -> None:
    digest = hashlib.sha256()
    with local_path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    expected_digest = digest.hexdigest()
    attempts = 5
    next_progress = 10
    for attempt in range(1, attempts + 1):
        try:
            transport = client.get_transport()
            if transport is None or not transport.is_active():
                connect_client(client, host, port, user, password)
            sftp = client.open_sftp()
            try:
                sftp.get_channel().settimeout(90)
                try:
                    offset = sftp.stat(remote_path).st_size
                except FileNotFoundError:
                    offset = 0
                if offset > size:
                    sftp.remove(remote_path)
                    offset = 0
                if offset:
                    print(f"Resuming upload at {offset / 1024 / 1024:.1f} MiB...", flush=True)
                if offset < size:
                    with local_path.open("rb") as source:
                        source.seek(offset)
                        with sftp.file(remote_path, "ab" if offset else "wb") as target:
                            target.set_pipelined(False)
                            sent = offset
                            while sent < size:
                                chunk = source.read(min(256 * 1024, size - sent))
                                if not chunk:
                                    raise DeployError("The local image archive ended unexpectedly.")
                                target.write(chunk)
                                sent += len(chunk)
                                percent = sent * 100 // size
                                if percent >= next_progress:
                                    print(f"Upload: {min(percent, 100)}%", flush=True)
                                    next_progress = (percent // 10 + 1) * 10
                actual_size = sftp.stat(remote_path).st_size
                if actual_size != size:
                    raise OSError(f"Remote archive size mismatch: expected {size}, got {actual_size}.")
            finally:
                try:
                    sftp.close()
                except (EOFError, OSError, paramiko.SSHException):
                    pass
            checksum_output = remote(client, f"sha256sum {shlex.quote(remote_path)}").split()
            remote_digest = checksum_output[0] if checksum_output else ""
            if remote_digest != expected_digest:
                remote(client, f"rm -f {shlex.quote(remote_path)}")
                next_progress = 10
                raise OSError("Uploaded archive checksum mismatch; restarting the upload.")
            print("Upload checksum verified.", flush=True)
            return
        except (DeployError, EOFError, OSError, paramiko.SSHException) as exc:
            if attempt == attempts:
                raise DeployError("Upload failed after five attempts. The server app was not restarted.") from exc
            print(f"Upload interrupted; reconnecting ({attempt}/{attempts - 1})...", flush=True)
            client.close()
            time.sleep(2 * attempt)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--user", required=True)
    parser.add_argument("--port", type=int, default=22)
    parser.add_argument("--remote-dir", required=True)
    parser.add_argument("--credential-file", type=Path, required=True)
    parser.add_argument("--check-only", action="store_true", help="Verify local and remote prerequisites without deploying")
    args = parser.parse_args()
    if not args.remote_dir.startswith("/"):
        raise DeployError("The remote directory must be an absolute Linux path.")

    local_docker_check()
    password = read_password(args.credential_file, args.user, args.host)
    client = paramiko.SSHClient()
    client.load_system_host_keys()
    client.set_missing_host_key_policy(paramiko.RejectPolicy())
    try:
        connect_client(client, args.host, args.port, args.user, password)
        platform = preflight(client, args.remote_dir)
        if args.check_only:
            print("Checks passed. No image was built or uploaded.")
        else:
            deploy(
                client,
                platform,
                args.remote_dir,
                host=args.host,
                port=args.port,
                user=args.user,
                password=password,
            )
        return 0
    finally:
        client.close()


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (DeployError, EOFError, paramiko.SSHException, OSError) as exc:
        print(f"Deployment stopped: {exc}", file=sys.stderr)
        sys.exit(1)
