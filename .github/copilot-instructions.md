# Trellis-CLI WSL2 Fork — Workspace Instructions

## Project Context
This is a fork of [roots/trellis-cli](https://github.com/roots/trellis-cli) adding a native **WSL2 virtual machine manager** for Windows users. The upstream CLI supports Lima (macOS/Linux). Our fork adds a `wsl` backend that manages WSL2 distros via `wsl.exe`, giving Windows developers a first-class Trellis development experience.

- **Language:** Go (module: `github.com/roots/trellis-cli`)
- **Fork:** https://github.com/qwatts-dev/trellis-cli
- **Upstream:** https://github.com/roots/trellis-cli

## Architecture

### VM Manager Interface (`pkg/vm/vm.go`)
All VM backends implement `vm.Manager`. Our WSL backend lives in `pkg/wsl/`.

```go
type Manager interface {
    CreateInstance(name string) error
    DeleteInstance(name string) error
    InventoryPath() string
    StartInstance(name string) error
    StopInstance(name string) error
    OpenShell(name string, dir string, commandArgs []string) error
    RunCommand(args []string, dir string) error
    RunCommandPipe(args []string, dir string) (*exec.Cmd, error)
}
```

### Key Files

| File | Purpose |
|---|---|
| `pkg/wsl/manager.go` | Core WSL2 Manager — all vm.Manager methods + Bootstrap + Provision + SyncBack + TrustSslCerts + syncConfigFromWSL + DecodeWslOutput + stopOtherDistros + syncBackDistro |
| `pkg/wsl/hosts.go` | WindowsHostsResolver — manages Windows hosts file with UAC elevation |
| `pkg/wsl/ubuntu.go` | Ubuntu rootfs URL registry (22.04, 24.04) |
| `cmd/vm.go` | `newVmManager()` switch — `case "wsl"` + `wslTerminalRequired()` + `windowsHostRequired()` guards |
| `cmd/vm_open.go` | Opens VS Code in WSL via `--folder-uri vscode-remote://wsl+<distro>/path` |
| `cmd/vm_sync.go` | Manual WSL→Windows rsync sync |
| `cmd/vm_trust.go` | Re-imports SSL certs into Windows trust store |
| `cmd/vm_start.go` | WSL bootstrap/provision flow, unprovisioned cleanup |
| `cmd/vm_stop.go` | Auto SyncBack before stop |
| `trellis/trellis.go` | WSL auto-detection in `VmManagerType()`, `ReloadSiteConfigs()`, `CheckVirtualenv` skip |
| `pkg/db_opener/tableplus.go` | `rundll32.exe` for Windows/WSL URI opening, direct `mysql://` for WSL |

### How It Works

- **Distro naming**: `trellis-` prefix + dots→hyphens (e.g. `example.com` → `trellis-example-com`)
- **Project on ext4**: Entire project rsync'd to `/home/admin/<project>/` during bootstrap. `site/` bind-mounted to `/srv/www/<name>/current` via fstab. ~80ms TTFB vs ~14s with DrvFS.
- **Inventory**: `ansible_connection=local`, `ansible_user=admin` (no SSH needed)
- **Keepalive**: `wsl --exec sleep infinity` from Windows keeps distro alive (systemd services alone don't prevent WSL idle shutdown)
- **Bootstrap installs**: Python, Ansible, Node.js LTS, Corepack (yarn/pnpm), rsync
- **One project at a time**: All WSL2 distros share a single network namespace ([MS by-design](https://github.com/microsoft/WSL/issues/4304)). `StartInstance` calls `stopOtherDistros()` which prompts to SyncBack other running `trellis-*` distros before terminating them.
- **openssh-server prevention**: Bootstrap creates `/etc/ssh/sshd_not_to_be_run` so ssh.socket never claims port 22 (we use local connection, not SSH)
- **Breadcrumb file**: Bootstrap writes `/etc/trellis-project-root` (Windows path) so `syncBackDistro()` works for any distro without loading its trellis config
- **Two guard functions**: `wslTerminalRequired()` (redirects Ansible commands from Windows → WSL) and `windowsHostRequired()` (redirects VM management from WSL → Windows)
- **Config sync**: `syncConfigFromWSL()` runs in `NewManager()` when distro is running — rsyncs group_vars/ from ext4→Windows, then `ReloadSiteConfigs()` re-parses in-memory config
- **SSL trust**: Only processes sites with `ssl.enabled: true`. Uses certutil via UAC PowerShell elevation.
- **TablePlus**: `rundll32.exe url.dll,FileProtocolHandler` for URI opening. Direct `mysql://127.0.0.1:3306` (no SSH tunnels needed).
- **UTF-16LE**: `DecodeWslOutput()` handles wsl.exe UTF-16LE BOM + null-byte pairs

### WSL2 Command Mapping
| Method | wsl.exe Command |
|---|---|
| `CreateInstance` | `wsl --import <name> <installDir> <tarball>` |
| `StartInstance` | `wsl --exec sleep infinity` (keepalive) |
| `StopInstance` | `wsl -t <name>` |
| `DeleteInstance` | `wsl --unregister <name>` |
| `OpenShell` | `wsl -d <name> --cd <dir> -- <command>` |
| `RunCommand` | `wsl -d <name> --cd <dir> -- <args...>` |

## Build & Test

```powershell
# Build both binaries (from the repo root)
go vet ./...; go build -o trellis.exe .
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o trellis-linux .; Remove-Item Env:GOOS; Remove-Item Env:GOARCH

# Test against a project (use the locally compiled binary, not the global trellis)
cd path\to\your\trellis-project
path\to\trellis-cli\trellis.exe vm start
```

**Important:** Always build BOTH `trellis.exe` (Windows) and `trellis-linux` (cross-compiled for WSL distros). The Linux binary is copied into distros during bootstrap.

## Coding Conventions
- Follow existing code patterns in `pkg/lima/manager.go` as the reference implementation
- Use `github.com/roots/trellis-cli/command` package for exec (matches upstream style)
- Use `github.com/fatih/color` for colored output (matches upstream)
- Use `github.com/manifoldco/promptui` for interactive prompts (matches upstream pattern)
- Keep WSL-specific code in `pkg/wsl/` — do not scatter Windows logic elsewhere
- Run `go vet ./...` before committing

## Key Gotchas
- **`/mnt/c/` = 777 permissions**: `.vault_pass` must be copied inside distro with `chmod 600`
- **Do NOT use `fmask=0111` in wsl.conf**: Breaks VS Code WSL extension (`wslServer.sh: Permission denied`)
- **rsync to DrvFS**: Use `-rlpt` not `-a` (chgrp fails). Use `--chmod=D755,F644` for Windows→WSL copies.
- **`cmd /c start` can't handle `&` in URIs**: Use `rundll32 url.dll,FileProtocolHandler` instead
- **PowerShell garbles UTF-8 multibyte**: Use `[ok]` text not `✓` checkmarks
- **`os.Rename` fails on Windows**: Antivirus file locks → retry loop + early file handle close in `github/main.go`
- **WSL2 shared network namespace**: All distros share IP+ports. Only one trellis distro can run services at a time.
- **Node.js on WSL not host**: Unlike upstream Lima (Node on macOS host), WSL project files live on ext4 so Node/yarn must be inside the distro.

## Testing
- **OS:** Windows 11 with WSL2 enabled
- **Isolation:** If you have a global trellis-cli install, do NOT run the locally compiled binary from your PATH. Always invoke it by its full path to avoid conflicts.
