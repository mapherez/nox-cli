#!/usr/bin/env python3
"""Build installer inputs from this checkout; never publish or tag anything."""
import argparse
import datetime
import hashlib
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parent.parent
TARGETS = [(system, arch) for system in ('darwin', 'linux', 'windows') for arch in ('amd64', 'arm64')]
SCRIPTS = ('install.sh', 'install.ps1', 'uninstall.sh', 'uninstall.ps1')


def verify(directory):
    expected = set(SCRIPTS) | {'checksums.txt'}
    for system, arch in TARGETS:
        expected.add(f'nox-cli_{system}_{arch}' + ('.zip' if system == 'windows' else '.tar.gz'))
    if {p.name for p in directory.iterdir()} != expected:
        raise ValueError('Release files are missing or unexpected.')
    records = {}
    for line in (directory / 'checksums.txt').read_text().splitlines():
        digest, name = line.split('  ')
        if name in records or not re.fullmatch('[0-9a-f]{64}', digest):
            raise ValueError('Invalid checksum manifest.')
        records[name] = digest
    if set(records) != expected - {'checksums.txt'}:
        raise ValueError('Incomplete checksum manifest.')
    for name, digest in records.items():
        if hashlib.sha256((directory / name).read_bytes()).hexdigest() != digest:
            raise ValueError('Release checksum mismatch.')
    for system, arch in TARGETS:
        binary = 'nox.exe' if system == 'windows' else 'nox'
        name = f'nox-cli_{system}_{arch}'
        if system == 'windows':
            with zipfile.ZipFile(directory / (name + '.zip')) as archive:
                if archive.namelist() != [binary] or not archive.read(binary).startswith(b'MZ'):
                    raise ValueError('Invalid Windows package.')
        else:
            with tarfile.open(directory / (name + '.tar.gz')) as archive:
                members = archive.getmembers()
                if len(members) != 1 or members[0].name != binary or not members[0].isreg():
                    raise ValueError('Invalid POSIX package.')
                payload = archive.extractfile(members[0]).read(4)
                if system == 'linux' and payload != b'\x7fELF':
                    raise ValueError('Invalid Linux executable.')
    print('PASS: all six packages, four scripts and complete SHA-256 manifest.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version')
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--verify', action='store_true')
    args = parser.parse_args()
    if args.verify:
        verify(args.output)
        return
    if not args.version or not re.fullmatch(r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', args.version):
        parser.error('A stable release tag vX.Y.Z is required.')
    if args.output.exists():
        parser.error('The output directory must not exist; existing files are never deleted.')
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    # Release workflow checks out the exact tag; local invocation only builds fixtures.
    date = datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    flags = ' '.join(f'-X github.com/mapherez/nox-cli/internal/buildinfo.{key}={value}'
                     for key, value in [('Version', args.version), ('Commit', commit), ('Date', date)])
    with tempfile.TemporaryDirectory(prefix='nox-cli-release-') as temporary:
        staging = Path(temporary)
        for system, arch in TARGETS:
            binary = 'nox.exe' if system == 'windows' else 'nox'
            executable = staging / binary
            subprocess.run(['go', 'build', '-mod=readonly', '-trimpath', '-ldflags', flags,
                            '-o', str(executable), './cmd/nox'], cwd=ROOT,
                           env=dict(os.environ, GOOS=system, GOARCH=arch, CGO_ENABLED='0'), check=True)
            name = f'nox-cli_{system}_{arch}'
            if system == 'windows':
                with zipfile.ZipFile(staging / (name + '.zip'), 'w', zipfile.ZIP_DEFLATED) as archive:
                    archive.write(executable, binary)
            else:
                with tarfile.open(staging / (name + '.tar.gz'), 'w:gz') as archive:
                    archive.add(executable, arcname=binary)
            executable.unlink()
        for name in SCRIPTS:
            shutil.copyfile(ROOT / name, staging / name)
        sums = ''.join(hashlib.sha256(p.read_bytes()).hexdigest() + '  ' + p.name + '\n'
                       for p in sorted(staging.iterdir()))
        (staging / 'checksums.txt').write_text(sums)
        verify(staging)
        # Only make the destination visible once every build and checksum passed.
        shutil.copytree(staging, args.output)


if __name__ == '__main__':
    main()
