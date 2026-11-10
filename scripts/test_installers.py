"""POSIX installer checks using official URL shapes and disposable release fixtures.

The fixture executable checks the native handoff only. Go tests exercise actual
activation/PATH edits in isolated directories; these are not installed releases.
"""
import hashlib
import io
import os
from pathlib import Path
import shlex
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


@unittest.skipUnless(os.name == "posix", "POSIX installer; Windows has its own native checks")
class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="nox-cli-installer-test-")
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.assets = self.base / "assets"
        self.tools = self.base / "tools"
        self.assets.mkdir()
        self.tools.mkdir()
        self.marker = self.base / "activation-marker"
        self.data = self.base / "data with spaces"
        self.root = self.data / "nox-cli"
        self.env = dict(os.environ, XDG_DATA_HOME=str(self.data), SHELL="/bin/bash",
                        NOX_CLI_TEST_ASSETS=str(self.assets), NOX_CLI_TEST_ACTIVATED=str(self.marker))
        self.env["PATH"] = str(self.tools) + os.pathsep + self.env["PATH"]
        self.tool("uname", '#!/bin/sh\ncase "$1" in -s) echo Linux;; -m) echo x86_64;; esac\n')
        self.tool("curl", r'''#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in https://*) url="$1";; -o) shift; output="$1";; esac
 shift
done
case "$url" in https://github.com/mapherez/nox-cli/releases/latest/download/*) ;; *) exit 9;; esac
if [ "${NOX_CLI_TEST_INTERRUPT:-}" = 1 ]; then
 : > "$output"
 kill -TERM "$PPID"
 exit 1
fi
cp "$NOX_CLI_TEST_ASSETS/${url##*/}" "$output"
''')
        self.archive()

    def tool(self, name, content):
        path = self.tools / name
        path.write_text(content)
        path.chmod(0o700)

    def archive(self, name="nox", link=False, failure=False):
        path = self.assets / "nox-cli_linux_amd64.tar.gz"
        content = b'#!/bin/sh\n[ "$1" = --complete-install ] || exit 3\nprintf activated > "$NOX_CLI_TEST_ACTIVATED"\n'
        if failure:
            content = b"#!/bin/sh\nexit 1\n"
        with tarfile.open(path, "w:gz") as archive:
            info = tarfile.TarInfo(name)
            info.mode = 0o700
            if link:
                info.type = tarfile.SYMTYPE
                info.linkname = "/bin/sh"
                archive.addfile(info)
            else:
                info.size = len(content)
                archive.addfile(info, io.BytesIO(content))
        checksum = hashlib.sha256(path.read_bytes()).hexdigest()
        (self.assets / "checksums.txt").write_text(checksum + "  " + path.name + "\n")

    def run_installer(self):
        return subprocess.run(["sh", str(ROOT / "install.sh")], cwd=self.base, env=self.env,
                              capture_output=True, text=True, timeout=15)

    def clean(self):
        self.assertFalse(self.root.exists(), "owned installer staging was left behind")

    def test_verified_handoff_and_cleanup(self):
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.marker.read_text(), "activated")
        self.clean()

    def test_bad_checksum_never_executes(self):
        (self.assets / "checksums.txt").write_text("0" * 64 + "  nox-cli_linux_amd64.tar.gz\n")
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.marker.exists())
        self.clean()

    def test_traversal_and_links_are_rejected(self):
        for name, link in [("../escaped", False), ("nox", True)]:
            self.archive(name=name, link=link)
            result = self.run_installer()
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(self.marker.exists())
            self.assertFalse((self.data / "escaped").exists())
            self.clean()

    def test_failed_activation_cleans_downloads(self):
        self.archive(failure=True)
        self.assertNotEqual(self.run_installer().returncode, 0)
        self.clean()

    def test_interruption_cleans_downloads(self):
        self.env["NOX_CLI_TEST_INTERRUPT"] = "1"
        self.assertNotEqual(self.run_installer().returncode, 0)
        self.clean()

    def test_stale_download_lock_is_recovered(self):
        lock = self.root / "install" / "installer.lock"
        lock.mkdir(parents=True)
        (lock / "pid").write_text("999999999\n")
        (self.root / "install" / "payload.tar.gz").write_text("interrupted")
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.clean()

    def test_interrupted_empty_lock_is_recovered(self):
        lock = self.root / "install" / "installer.lock"
        lock.mkdir(parents=True)
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.clean()

    def test_linked_root_is_not_touched(self):
        victim = self.base / "unrelated"
        victim.mkdir()
        original = victim / "keep"
        original.write_text("untouched")
        self.data.mkdir()
        self.root.symlink_to(victim, target_is_directory=True)
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(original.read_text(), "untouched")
        self.assertTrue(self.root.is_symlink())


@unittest.skipUnless(os.name == 'posix', 'Native POSIX script lifecycle')
class NativeLifecycleTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.directory = tempfile.TemporaryDirectory(prefix='nox-cli-native-install-')
        cls.addClassCleanup(cls.directory.cleanup)
        cls.binary = Path(cls.directory.name) / 'nox'
        subprocess.run(['go', 'build', '-ldflags',
                        '-X github.com/mapherez/nox-cli/internal/buildinfo.Version=v0.1.0',
                        '-o', str(cls.binary), './cmd/nox'], cwd=ROOT, check=True, timeout=120)

    def test_native_install_reinstall_remove_without_running_installed_binary(self):
        import platform
        with tempfile.TemporaryDirectory(prefix='nox-cli-lifecycle-') as temporary:
            base = Path(temporary)
            home = base / 'user with spaces'
            home.mkdir()
            tools, assets = base / 'tools', base / 'assets'
            tools.mkdir()
            assets.mkdir()
            system = 'darwin' if platform.system() == 'Darwin' else 'linux'
            arch = 'arm64' if platform.machine().lower() in ('arm64', 'aarch64') else 'amd64'
            asset = assets / f'nox-cli_{system}_{arch}.tar.gz'
            with tarfile.open(asset, 'w:gz') as archive:
                archive.add(self.binary, arcname='nox')
            (assets / 'checksums.txt').write_text(hashlib.sha256(asset.read_bytes()).hexdigest() + '  ' + asset.name + '\n')
            (assets / 'install.sh').write_bytes((ROOT / 'install.sh').read_bytes())
            curl = tools / 'curl'
            curl.write_text('''#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in https://*) url="$1";; -o) shift; output="$1";; esac
 shift
done
case "$url" in https://github.com/mapherez/nox-cli/releases/latest/download/*) ;; *) exit 9;; esac
cp "$NOX_CLI_TEST_ASSETS/${url##*/}" "$output"
''')
            curl.chmod(0o700)
            env = dict(os.environ, HOME=str(home), XDG_DATA_HOME=str(home / 'data'),
                       SHELL='/bin/bash', PATH=str(tools) + os.pathsep + os.environ['PATH'],
                       NOX_CLI_TEST_ASSETS=str(assets))
            root = home / 'Library/Application Support/nox-cli' if system == 'darwin' else home / 'data/nox-cli'
            for _ in range(2):
                installed = subprocess.run(['sh', str(ROOT / 'install.sh')], env=env, capture_output=True, text=True, timeout=30)
                self.assertEqual(installed.returncode, 0, installed.stderr)
                self.assertTrue((root / 'install-state.json').exists())
                self.assertFalse((root / 'install').exists())
            self.assertEqual((home / '.bashrc').read_text().count('# >>> nox-cli installer >>>'), 1)
            # Persist only non-secret data; the default uninstall must never open a vault.
            (root / 'config.json').write_text('preserved configuration')
            (root / 'unrelated').write_text('keep me')
            # A non-executable installed payload still has its verified ownership hash.
            (root / 'nox').chmod(0o600)
            removed = subprocess.run(['sh', str(ROOT / 'uninstall.sh')], env=env, capture_output=True, text=True, timeout=30)
            self.assertEqual(removed.returncode, 0, removed.stderr)
            self.assertFalse((root / 'nox').exists())
            self.assertFalse((root / 'install-state.json').exists())
            self.assertFalse((root / 'install').exists())
            self.assertEqual((root / 'config.json').read_text(), 'preserved configuration')
            self.assertEqual((root / 'unrelated').read_text(), 'keep me')
            self.assertFalse((home / '.bashrc').exists(), 'Installer-created empty profile was left behind')


if __name__ == "__main__":
    unittest.main()
