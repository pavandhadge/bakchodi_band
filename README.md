# bakchodi_band

`bakchodi_band` is a local attention blocker. The command users run is `bakchodi`.

The app runs as a background system service, edits the system hosts file, and keeps distracting sites blocked. Users do not normally stop the blocker. They ask for short breaks through the CLI, and those breaks include friction: wait time, hard math, a daily time budget, reason logging, commitments, and emergency break-glass logging.

Default policy:

- Each normal break: `25` minutes
- Maximum normal break time per day: `125` minutes
- Emergency break-glass time: `5` minutes

## Commands

Start the background blocker manually:

```bash
sudo bakchodi band
```

Block sites:

```bash
bakchodi block --url youtube.com
bakchodi block --group social
bakchodi block --all
```

Ask for a normal break:

```bash
bakchodi allow --url youtube.com
bakchodi allow --group social
bakchodi allow --all
```

Emergency break:

```bash
bakchodi panic --url youtube.com
```

Create a no-break commitment window:

```bash
bakchodi plan --hours 24 --reason "exam tomorrow"
```

Create groups:

```bash
bakchodi add-group --name social --url x.com --url reddit.com
bakchodi add-group --name social --file social.txt
```

Generate and import a full setup file:

```bash
bakchodi sample-config --out bakchodi.config.json
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

### Non-Technical Install

After GitHub Releases are published, the easiest flow should be a single command copied into Terminal.

Linux/macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/pavandhadge/bakchodi_band/main/packaging/install-release.sh | sudo sh
```

If `curl` is not available:

```bash
wget -qO- https://raw.githubusercontent.com/pavandhadge/bakchodi_band/main/packaging/install-release.sh | sudo sh
```

Windows PowerShell as Administrator:

```powershell
irm https://raw.githubusercontent.com/pavandhadge/bakchodi_band/main/packaging/windows/install-release.ps1 | iex
```

Those release installers should download the correct `bakchodi` binary for the OS, place it in a system location, register the background service, and start it.

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

### Compile Yourself

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

