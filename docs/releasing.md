# Releasing

Mimir release builds use GitHub Releases as the update source.

## Versioning

Use semantic versions without the leading `v` in build scripts:

```powershell
.\scripts\release-all.ps1 -Version 0.1.0 -UpdateRepository OWNER/REPO
```

Create the GitHub tag with a leading `v`:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Local Builds

Windows and Linux from Windows PowerShell with WSL:

```powershell
.\scripts\release-all.ps1 -Version 0.1.0 -UpdateRepository OWNER/REPO
```

Linux from Linux:

```bash
./scripts/release-linux.sh 0.1.0 --update-repo=OWNER/REPO
```

macOS from macOS:

```bash
./scripts/release-macos.sh 0.1.0 --update-repo=OWNER/REPO
```

Artifacts are written to `build/release/` together with `checksums.txt`.

## GitHub Actions

The release workflow runs on tags matching `v*`.

It builds platform artifacts, writes checksums, and uploads them to the GitHub Release.

## Update Checks

Release builds embed:

- `main.AppVersion`
- `main.UpdateRepository`

The app checks:

```text
https://api.github.com/repos/OWNER/REPO/releases/latest
```

The current MVP only opens the release page. It does not replace binaries automatically.
