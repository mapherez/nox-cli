#!/usr/bin/env python3
"""Exercise compiled process behavior, POSIX terminal cleanup, or target builds.

All executable outputs live in TemporaryDirectory and are removed on completion.
The in-memory MCP client test is part of go test, not a product simulator.
"""

import argparse
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
TARGETS = [(system, arch) for system in ("windows", "darwin", "linux") for arch in ("amd64", "arm64")]
BUILD_VERSION = "foundation-check"
LDFLAGS = " ".join([
    "-X github.com/mapherez/nox-cli/internal/buildinfo.Version=" + BUILD_VERSION,
    "-X github.com/mapherez/nox-cli/internal/buildinfo.Commit=test-commit",
    "-X github.com/mapherez/nox-cli/internal/buildinfo.Date=2026-11-04T00:00:00Z",
])


def build(path, env=None):
    subprocess.run(["go", "build", "-mod=readonly", "-trimpath", "-ldflags", LDFLAGS,
                    "-o", str(path), "./cmd/nox"], cwd=ROOT, env=env, check=True, timeout=300)


def process_checks(binary):
    for args, code, expected in [
        (["--version"], 0, "nox foundation-check (commit test-commit, built 2026-11-04T00:00:00Z)"),
        (["--help"], 0, "Usage: nox"),
        ([], 2, "An interactive terminal is required"),
        (["not-a-command"], 2, "Unsupported arguments"),
    ]:
        result = subprocess.run([str(binary), *args], cwd=ROOT, input="", capture_output=True,
                                text=True, timeout=10)
        output = result.stdout if code == 0 else result.stderr
        assert result.returncode == code and expected in output, (args, result)
        assert not (result.stderr if code == 0 else result.stdout), (args, result)
    print("PASS: compiled version injection, help, argument errors, and non-interactive rejection", flush=True)


def terminal_check(binary, action):
    # Import only on POSIX; Windows process/build checks still run natively.
    import fcntl
    import select
    import signal
    import struct
    import termios
    import time

    master, slave = os.openpty()
    before = termios.tcgetattr(slave)
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 90, 0, 0))
    env = dict(os.environ, TERM="xterm-256color")
    isolated = tempfile.TemporaryDirectory(prefix="nox-cli-terminal-")
    # Set all platform roots; config.Root selects the current platform's one.
    env.update(HOME=isolated.name, XDG_DATA_HOME=isolated.name, LOCALAPPDATA=isolated.name)
    process = None
    output = bytearray()

    def receive(timeout):
        if select.select([master], [], [], timeout)[0]:
            output.extend(os.read(master, 65536))

    try:
        process = subprocess.Popen([str(binary)], cwd=ROOT, stdin=slave, stdout=slave,
                                   stderr=slave, env=env, start_new_session=True)
        deadline = time.monotonic() + 10
        while b"Home" not in output:
            assert process.poll() is None, (action, process.returncode, bytes(output))
            assert time.monotonic() < deadline, ("terminal startup timed out", action, bytes(output))
            receive(0.1)
        assert b"NoX" in output and BUILD_VERSION.encode() in output, bytes(output)

        if action == "escape":
            # Exercise the real Settings screen without saving or touching a vault.
            os.write(master, b"\r")
            deadline = time.monotonic() + 5
            while b"Gateway:" not in output:
                assert time.monotonic() < deadline, "Settings did not open"
                receive(0.1)
            os.write(master, b"\tprivate-test-secret")
            deadline = time.monotonic() + 1
            while time.monotonic() < deadline:
                receive(0.05)
            assert b"private-test-secret" not in output, "password input leaked to the terminal"
            os.write(master, b"\x1b")
            # Esc is ambiguous with a sequence prefix, so wait for its dispatch.
            deadline = time.monotonic() + 1
            while time.monotonic() < deadline:
                receive(0.05)
        active = termios.tcgetattr(slave)
        assert not active[3] & termios.ECHO and not active[3] & termios.ICANON, "terminal was never in raw input mode"
        # A real resize must not break subsequent exit/cleanup.
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 18, 60, 0, 0))
        process.send_signal(signal.SIGWINCH)
        if action == "escape":
            os.write(master, b"\x1b")
        elif action == "ctrl-c":
            os.write(master, b"\x03")
        else:
            process.send_signal(getattr(signal, action))
        deadline = time.monotonic() + 10
        while process.poll() is None:
            assert time.monotonic() < deadline, ("terminal exit timed out", action, bytes(output))
            receive(0.1)
        while select.select([master], [], [], 0.05)[0]:
            receive(0)
        expected = 130 if action == "SIGINT" else 0
        assert process.returncode == expected, (action, process.returncode, bytes(output))
        after = termios.tcgetattr(slave)
        expected_attributes = before[:]
        if sys.platform == "darwin":
            # XNU adds PENDIN when restoring ICANON. A Python-only setraw /
            # tcsetattr round trip reproduces this without running the CLI.
            # Permit only that kernel-added flag; retain every other check.
            # https://github.com/apple/darwin-xnu/blob/main/bsd/kern/tty.c
            expected_attributes[3] |= after[3] & termios.PENDIN
        assert after == expected_attributes, (action, "terminal attributes were not restored", [(index, old, new) for index, (old, new) in enumerate(zip(expected_attributes, after)) if old != new])
        assert b"\x1b[?1049h" in output and b"\x1b[?1049l" in output, (action, "alternate screen was not restored", bytes(output))
        assert output.rfind(b"\x1b[?25h") > output.rfind(b"\x1b[?25l"), (action, "cursor was not restored")
        print("PASS: real PTY startup, resize, exit and terminal restoration (" + action + ")", flush=True)
    finally:
        if process is not None:
            if process.poll() is None:
                process.kill()
            process.wait(timeout=5)
        termios.tcsetattr(slave, termios.TCSANOW, before)
        os.close(master)
        os.close(slave)
        isolated.cleanup()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cross-build", action="store_true", help="Compile all six planned targets; does not certify native runtime behavior.")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="nox-cli-foundation-") as directory:
        root = Path(directory)
        if args.cross_build:
            for system, arch in TARGETS:
                env = dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED="0")
                binary = root / (system + "-" + arch + (".exe" if system == "windows" else ""))
                build(binary, env)
                assert binary.stat().st_size > 0
                binary.unlink()
                print("PASS: build " + system + "/" + arch + " (compile only)", flush=True)
        else:
            binary = root / ("nox.exe" if os.name == "nt" else "nox")
            build(binary)
            process_checks(binary)
            if os.name == "posix":
                for action in ("escape", "ctrl-c", "SIGINT", "SIGTERM"):
                    terminal_check(binary, action)
            else:
                print("NOT EXERCISED: native Windows terminal restoration requires a ConPTY/manual check; POSIX PTY results do not cover it.", flush=True)


if __name__ == "__main__":
    main()
