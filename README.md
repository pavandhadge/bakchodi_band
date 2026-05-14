# bakchodi_band

`bakchodi_band` is a local attention blocker. The command users run is `bakchodi`.

The app runs as a background system service, edits the system hosts file, and keeps distracting sites blocked. Users do not normally stop the blocker. They ask for short breaks through the CLI, and those breaks include friction: wait time, hard math, a daily time budget, reason logging, commitments, and emergency break-glass logging.

Default policy:

- Each normal break: `25` minutes
- Maximum normal break time per day: `125` minutes
- Emergency break-glass time: `5` minutes

## Commands

### Command Basics

Most commands follow this pattern:

```bash
bakchodi <action> <target>
```

The action says what you want to do. The target says which sites it should affect.

Targets:

- `--url youtube.com` means one website only.
- `--group social` means a saved group of websites, such as `x.com`, `instagram.com`, and `reddit.com`.
- `--all` means every website in every saved group.

Use `--url` when one site is the problem. Use `--group` when a whole category is the problem. Use `--all` when you want the strictest mode and every saved distraction should be blocked.

### Start The Background Blocker

The installer starts the blocker automatically. Use this only if you are running it manually or debugging:

```bash
sudo bakchodi band
```

This command runs the background service that edits the hosts file and enforces blocks.

### Block Sites

Block one site:

```bash
bakchodi block --url youtube.com
```

This blocks only `youtube.com`.

Block a group:

```bash
bakchodi block --group social
```

This blocks every site saved inside the `social` group.

Block everything you have saved:

```bash
bakchodi block --all
```

This blocks all sites from all saved groups.

### Ask For A Normal Break

A normal break temporarily allows a blocked site or group. By default, one break lasts `25` minutes, and the normal daily limit is `125` minutes.

Temporarily allow one site:

```bash
bakchodi allow --url youtube.com
```

Temporarily allow a group:

```bash
bakchodi allow --group social
```

Temporarily allow every saved blocked site:

```bash
bakchodi allow --all
```

Use `allow` for planned breaks. The app will still add friction before allowing the break.

### Emergency Break

Use `panic` only when you urgently need access and normal breaks are not enough:

```bash
bakchodi panic --url youtube.com
```

By default, emergency access lasts `5` minutes and is logged separately.

### Create A No-Break Commitment

Use `plan` when you want to prevent normal breaks for a fixed period:

```bash
bakchodi plan --hours 24 --reason "exam tomorrow"
```

This creates a `24` hour commitment window. During that window, normal `allow` requests are blocked.

### Create Groups

Groups let you control many sites with one name.

Create a group by typing sites directly:

```bash
bakchodi add-group --name social --url x.com --url reddit.com
```

Now `bakchodi block --group social` affects both `x.com` and `reddit.com`.

Create a group from a text file:

```bash
bakchodi add-group --name social --file social.txt
```

The file should contain one site per line:

```text
x.com
instagram.com
facebook.com
reddit.com
```

### Import A Full Setup

Generate an example setup file:

```bash
bakchodi sample-config --out bakchodi.config.json
```

Edit that file, then import it:

```bash
bakchodi setup --file bakchodi.config.json
```

Example config:

```json
{
  "policy": {
    "daily_budget_minutes": 125,
    "unlock_minutes": 25,
    "break_glass_minutes": 5
  },
  "groups": {
    "social": ["x.com", "instagram.com", "facebook.com", "reddit.com"],
    "video": ["youtube.com", "netflix.com", "twitch.tv"],
    "shorts": ["tiktok.com", "youtube.com", "instagram.com"]
  }
}
```

Old aliases still work: `start`, `daemon`, `unlock`, `break-glass`, `commit`, `group`, `default-config`.

## Install

The intended install is permanent service mode:

1. Install once as Administrator/root.
2. The installer registers `bakchodi_band` as an OS service.
3. The service starts automatically whenever the computer turns on.
4. The user only runs `bakchodi allow ...` or `bakchodi panic ...` to request breaks.

The OS must ask for admin/root permission once during install. A CLI cannot safely grant itself permanent system privilege silently.

### One-Command Install From GitHub

This is the recommended install for most users. It downloads the installer from GitHub, downloads the correct release binary for the current OS/CPU, installs the background service, starts it, and runs a basic installation test.

Linux/macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/pavandhadge/bakchodi_band/main/packaging/install.sh | sudo sh
```

If `curl` is not available:

```bash
wget -qO- https://raw.githubusercontent.com/pavandhadge/bakchodi_band/main/packaging/install.sh | sudo sh
```

Windows PowerShell as Administrator:

```powershell
irm https://raw.githubusercontent.com/pavandhadge/bakchodi_band/main/packaging/install.ps1 | iex
```

Install a specific release tag:

Linux/macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/pavandhadge/bakchodi_band/main/packaging/install.sh | sudo BAKCHODI_VERSION=v0.1.0 sh
```

Windows PowerShell as Administrator:

```powershell
$env:BAKCHODI_VERSION = "v0.1.0"; irm https://raw.githubusercontent.com/pavandhadge/bakchodi_band/main/packaging/install.ps1 | iex
```

Install from a fork:

Linux/macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/YOUR_USER/bakchodi_band/main/packaging/install.sh | sudo BAKCHODI_REPO=YOUR_USER/bakchodi_band sh
```

Windows PowerShell as Administrator:

```powershell
$env:BAKCHODI_REPO = "YOUR_USER/bakchodi_band"; irm https://raw.githubusercontent.com/YOUR_USER/bakchodi_band/main/packaging/install.ps1 | iex
```

`packaging/install-release.sh` and `packaging/windows/install-release.ps1` remain as compatibility wrappers that fetch and run the canonical installers above.

### Install From GitHub Release Manually

Download the release file for your OS:

- Linux: `bakchodi-linux-amd64`
- macOS Intel: `bakchodi-darwin-amd64`
- macOS Apple Silicon: `bakchodi-darwin-arm64`
- Windows: `bakchodi-windows-amd64.exe`

Then run the matching installer script.

Linux:

```bash
chmod +x bakchodi-linux-amd64
sudo sh packaging/linux/install.sh ./bakchodi-linux-amd64
```

macOS:

```bash
chmod +x bakchodi-darwin-arm64
sudo sh packaging/macos/install.sh ./bakchodi-darwin-arm64
```

Windows PowerShell as Administrator:

```powershell
.\packaging\windows\install.ps1 -Binary .\bakchodi-windows-amd64.exe
```

### Compile And Install Yourself

Linux/macOS:

```bash
go build -o bakchodi ./cmd
sudo sh packaging/linux/install.sh ./bakchodi
```

On macOS use:

```bash
go build -o bakchodi ./cmd
sudo sh packaging/macos/install.sh ./bakchodi
```

Windows PowerShell as Administrator:

```powershell
go build -o bakchodi.exe ./cmd
.\packaging\windows\install.ps1 -Binary .\bakchodi.exe
```

### Verify Install

The one-command installers run these checks automatically. You can also run them yourself.

Check that the CLI is installed:

```bash
bakchodi
```

## Release

Release builds are produced with `make`. The generated asset names match the installers:

- `bakchodi-linux-amd64`
- `bakchodi-linux-arm64`
- `bakchodi-darwin-amd64`
- `bakchodi-darwin-arm64`
- `bakchodi-windows-amd64.exe`
- `bakchodi-windows-arm64.exe`

Build and test release assets locally:

```bash
make package VERSION=v0.1.0
```

Run release checks:

```bash
make release-check VERSION=v0.1.0
```

Publish a GitHub release after installing and authenticating the GitHub CLI:

```bash
gh auth login
make release VERSION=v0.1.0
```

If the assets are already built and you only want to publish:

```bash
make publish VERSION=v0.1.0
```

### Check Service Status

Linux:

```bash
systemctl status bakchodi.service
```

macOS:

```bash
sudo launchctl print system/com.bakchodi.band
```

Windows:

```powershell
Get-Service bakchodi_band
```

### Uninstall

Linux:

```bash
sudo sh packaging/linux/uninstall.sh
```

macOS:

```bash
sudo sh packaging/macos/uninstall.sh
```

Windows PowerShell as Administrator:

```powershell
.\packaging\windows\uninstall.ps1
```
