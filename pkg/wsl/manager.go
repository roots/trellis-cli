package wsl

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/manifoldco/promptui"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

const configDir = "wsl"

// printStatus prints a status message to the terminal, ensuring the cursor
// starts at column 0. On Windows with ENABLE_VIRTUAL_TERMINAL_PROCESSING
// (needed for ANSI colors), \n is a bare line feed that keeps the current
// column. Without an explicit \r, successive messages drift rightward.
func printStatus(ui cli.Ui, msg string) {
	fmt.Print("\r")
	ui.Info(msg)
}

// Manager implements vm.Manager for WSL2 on Windows.
//
// Each trellis project gets its own WSL distro, managed via wsl.exe.
// The distro is created by importing an Ubuntu rootfs tarball with
// `wsl --import`, and all lifecycle operations map to wsl.exe subcommands.
type Manager struct {
	ConfigPath    string
	HostsResolver *WindowsHostsResolver
	Sites         map[string]*trellis.Site
	ui            cli.Ui
	trellis       *trellis.Trellis
}

// NewManager creates a WSL Manager. This is the constructor called from
// cmd/vm.go when the user's config selects the "wsl" backend.
//
// Go pattern: constructors are regular functions (not methods) named
// New<Type>. They return (*Type, error) so callers can handle failure.
func NewManager(trellis *trellis.Trellis, ui cli.Ui) (*Manager, error) {
	wslConfigPath := filepath.Join(trellis.ConfigPath(), configDir)
	hostNames := trellis.Environments["development"].AllHosts()

	manager := &Manager{
		ConfigPath:    wslConfigPath,
		HostsResolver: NewWindowsHostsResolver(hostNames),
		Sites:         trellis.Environments["development"].WordPressSites,
		trellis:       trellis,
		ui:            ui,
	}

	if err := os.MkdirAll(manager.ConfigPath, 0755); err != nil {
		return nil, fmt.Errorf("could not create config directory: %v", err)
	}

	// If the distro is running, sync config from WSL ext4 back to the
	// Windows filesystem so this Manager (and any command using it) sees
	// the latest wordpress_sites.yml, vault.yml, etc.
	//
	// This handles the common flow: developer edits config inside WSL via
	// VS Code, then runs a Windows-side command like `vm trust` or `vm stop`.
	siteName, _, _ := trellis.MainSiteFromEnvironment("development")
	distro := distroName(siteName)

	if manager.distroRunning(distro) {
		manager.syncConfigFromWSL(distro)

		trellis.ReloadSiteConfigs()
		manager.Sites = trellis.Environments["development"].WordPressSites
		manager.HostsResolver = NewWindowsHostsResolver(
			trellis.Environments["development"].AllHosts(),
		)
	}

	return manager, nil
}

// distroName converts a trellis site name (e.g. "wordpress.test") to a
// WSL-safe distro name (e.g. "trellis-wordpress-test").
//
// WSL distro names cannot contain dots. The "trellis-" prefix prevents
// collisions with user-installed distros like "Ubuntu-24.04".
func distroName(name string) string {
	return "trellis-" + strings.ReplaceAll(name, ".", "-")
}

// ---------------------------------------------------------------------------
// vm.Manager interface implementation
// ---------------------------------------------------------------------------

func (m *Manager) InventoryPath() string {
	return filepath.Join(m.ConfigPath, "inventory")
}

func (m *Manager) CreateInstance(name string) error {
	distro := distroName(name)

	if m.distroExists(distro) {
		printStatus(m.ui, fmt.Sprintf("WSL distro '%s' already exists.", distro))
		return nil
	}

	// Download Ubuntu rootfs tarball if not already cached.
	tarball, err := m.ensureRootfs()
	if err != nil {
		return err
	}

	// Each distro gets its own directory for the virtual disk.
	installDir := filepath.Join(m.ConfigPath, distro)
	if err = os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("could not create install directory: %v", err)
	}

	printStatus(m.ui, fmt.Sprintf("Importing WSL distro '%s'...", distro))

	err = command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("wsl", []string{"--import", distro, installDir, tarball}).Run()

	if err != nil {
		return fmt.Errorf("could not import WSL distro: %v", err)
	}

	if err = m.writeInventory(); err != nil {
		return err
	}

	printStatus(m.ui, fmt.Sprintf("%s WSL distro '%s' created", color.GreenString("[ok]"), distro))
	return nil
}

func (m *Manager) DeleteInstance(name string) error {
	distro := distroName(name)

	if !m.distroExists(distro) {
		printStatus(m.ui, "WSL distro does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	printStatus(m.ui, fmt.Sprintf("Unregistering WSL distro '%s'...", distro))

	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("wsl", []string{"--unregister", distro}).Run()

	if err != nil {
		return fmt.Errorf("could not unregister WSL distro: %v", err)
	}

	// Remove site hostnames from the Windows hosts file.
	if err := m.HostsResolver.RemoveHosts(distro); err != nil {
		m.ui.Warn(fmt.Sprintf("Warning: could not remove hosts entry: %v", err))
	}

	// Clean up the distro's virtual disk directory and provisioning marker.
	installDir := filepath.Join(m.ConfigPath, distro)
	os.RemoveAll(installDir)
	os.Remove(filepath.Join(m.ConfigPath, distro+".provisioned"))

	printStatus(m.ui, fmt.Sprintf("%s WSL distro '%s' deleted", color.GreenString("[ok]"), distro))
	return nil
}

func (m *Manager) StartInstance(name string) error {
	distro := distroName(name)

	if !m.distroExists(distro) {
		return vm.ErrVmNotFound
	}

	if m.distroRunning(distro) {
		printStatus(m.ui, fmt.Sprintf("%s WSL distro already running", color.GreenString("[ok]")))
		return nil
	}

	// Stop other trellis-* distros. All WSL2 distros share the same network
	// namespace (by design — one VM, one network stack), so services like
	// MariaDB (3306), nginx (80/443), and PHP-FPM collide if multiple
	// distros run simultaneously.
	m.stopOtherDistros(distro)

	// Start the distro with a keepalive process. WSL2 terminates distros when
	// no user processes are running under PID 2 (the WSL init). systemd services
	// (PID 1) do NOT prevent shutdown. Using `wsl --exec` from the Windows side
	// creates a process under PID 2, keeping the VM alive indefinitely.
	// See: https://github.com/microsoft/WSL/issues/10138
	//
	// `sleep infinity` is universally available in all Ubuntu rootfs images,
	// including fresh imports before bootstrap installs any packages.
	cmd := exec.Command("wsl", "-d", distro, "--exec", "sleep", "infinity")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start WSL distro keepalive: %v", err)
	}
	// Detach — the process outlives trellis-cli.
	go func() { _ = cmd.Wait() }()

	if err := m.writeInventory(); err != nil {
		return err
	}

	// Add site hostnames to the Windows hosts file (127.0.0.1) so the
	// browser can reach the dev site. WSL2 NAT forwards localhost ports
	// into the distro automatically.
	if err := m.HostsResolver.AddHosts(distro, "127.0.0.1"); err != nil {
		return err
	}

	printStatus(m.ui, fmt.Sprintf("%s WSL distro '%s' started", color.GreenString("[ok]"), distro))
	return nil
}

func (m *Manager) StopInstance(name string) error {
	distro := distroName(name)

	if !m.distroExists(distro) {
		printStatus(m.ui, "WSL distro does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if !m.distroRunning(distro) {
		printStatus(m.ui, fmt.Sprintf("%s WSL distro already stopped", color.GreenString("[ok]")))
		return nil
	}

	// `wsl -t <name>` terminates (stops) the distro.
	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("wsl", []string{"-t", distro}).Run()

	if err != nil {
		return fmt.Errorf("could not stop WSL distro: %v", err)
	}

	printStatus(m.ui, fmt.Sprintf("%s WSL distro '%s' stopped", color.GreenString("[ok]"), distro))
	return nil
}

func (m *Manager) OpenShell(name string, dir string, commandArgs []string) error {
	distro := distroName(name)

	if !m.distroExists(distro) {
		printStatus(m.ui, "WSL distro does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	// Ensure the distro is running. WSL2 may auto-shutdown idle distros even
	// with systemd enabled. Starting it is fast and idempotent.
	_ = command.Cmd("wsl", []string{"-d", distro, "--", "/bin/true"}).Run()

	args := []string{"-d", distro}
	if dir != "" {
		args = append(args, "--cd", dir)
	}
	if len(commandArgs) > 0 {
		args = append(args, "--")
		args = append(args, commandArgs...)
	}

	return command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("wsl", args).Run()
}

func (m *Manager) RunCommand(args []string, dir string) error {
	instanceName, err := m.trellis.GetVmInstanceName()
	if err != nil {
		return err
	}

	distro := distroName(instanceName)

	if !m.distroExists(distro) {
		return fmt.Errorf("WSL distro does not exist. Run `trellis vm start` to create it.")
	}

	// Ensure the distro is running (WSL2 may auto-shutdown idle distros).
	_ = command.Cmd("wsl", []string{"-d", distro, "--", "/bin/true"}).Run()

	wslArgs := []string{"-d", distro}
	if dir != "" {
		wslArgs = append(wslArgs, "--cd", dir)
	}
	wslArgs = append(wslArgs, "--")
	wslArgs = append(wslArgs, args...)

	return command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("wsl", wslArgs).Run()
}

// RunCommandPipe returns an *exec.Cmd that is ready to run but NOT yet started.
//
// Go pattern: returning an *exec.Cmd lets callers wire up their own
// stdin/stdout/stderr pipes before calling cmd.Start() + cmd.Wait().
// This is used by the logs command to stream output.
func (m *Manager) RunCommandPipe(args []string, dir string) (*exec.Cmd, error) {
	instanceName, err := m.trellis.GetVmInstanceName()
	if err != nil {
		return nil, err
	}

	distro := distroName(instanceName)

	if !m.distroExists(distro) {
		return nil, fmt.Errorf("WSL distro does not exist. Run `trellis vm start` to create it.")
	}

	// Ensure the distro is running (WSL2 may auto-shutdown idle distros).
	_ = command.Cmd("wsl", []string{"-d", distro, "--", "/bin/true"}).Run()

	wslArgs := []string{"-d", distro}
	if dir != "" {
		wslArgs = append(wslArgs, "--cd", dir)
	}
	wslArgs = append(wslArgs, "--")
	wslArgs = append(wslArgs, args...)

	return command.Cmd("wsl", wslArgs), nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// distroExists checks whether a WSL distro is registered.
// Parses the output of `wsl -l -q` (one distro name per line).
func (m *Manager) distroExists(distro string) bool {
	output, err := command.Cmd("wsl", []string{"-l", "-q"}).Output()
	if err != nil {
		return false
	}

	// wsl.exe outputs UTF-16LE on Windows — decode before parsing.
	decoded := DecodeWslOutput(output)

	for _, line := range strings.Split(decoded, "\n") {
		if strings.TrimSpace(line) == distro {
			return true
		}
	}
	return false
}

// distroRunning checks whether a WSL distro is currently in the Running state.
// Uses `wsl -l --running -q` which only lists running distros.
func (m *Manager) distroRunning(distro string) bool {
	output, err := command.Cmd("wsl", []string{"-l", "--running", "-q"}).Output()
	if err != nil {
		return false
	}

	decoded := DecodeWslOutput(output)

	for _, line := range strings.Split(decoded, "\n") {
		if strings.TrimSpace(line) == distro {
			return true
		}
	}
	return false
}

// stopOtherDistros terminates any running trellis-* WSL distros other than
// the one being started. All WSL2 distros share a single network namespace,
// so services (MariaDB 3306, nginx 80/443) from one distro block the same
// ports in every other distro.
func (m *Manager) stopOtherDistros(current string) {
	output, err := command.Cmd("wsl", []string{"-l", "--running", "-q"}).Output()
	if err != nil {
		return
	}

	decoded := DecodeWslOutput(output)

	for _, line := range strings.Split(decoded, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || name == current {
			continue
		}
		if !strings.HasPrefix(name, "trellis-") {
			continue
		}

		// Offer to SyncBack before stopping. The user may have unsaved
		// work in the other distro's ext4 filesystem that hasn't been
		// synced to the Windows side yet.
		prompt := promptui.Prompt{
			Label:     fmt.Sprintf("SyncBack '%s' before stopping", name),
			IsConfirm: true,
		}
		if _, promptErr := prompt.Run(); promptErr == nil {
			m.syncBackDistro(name)
		}

		printStatus(m.ui, fmt.Sprintf("Stopping '%s' (WSL distros share ports)...", name))
		_ = command.Cmd("wsl", []string{"--terminate", name}).Run()
	}
}

// syncBackDistro syncs project files from a running distro back to Windows.
// It reads the Windows project root from /etc/trellis-project-root (written
// during bootstrap) so it works for any distro, not just the current project.
func (m *Manager) syncBackDistro(distro string) {
	// Read the breadcrumb file that stores the Windows project root.
	raw, err := command.Cmd("wsl", []string{
		"-d", distro, "--", "cat", "/etc/trellis-project-root",
	}).Output()
	if err != nil {
		m.ui.Warn(fmt.Sprintf("Warning: could not read project root from '%s': %v", distro, err))
		return
	}

	projectRoot := strings.TrimSpace(string(raw))
	if projectRoot == "" {
		m.ui.Warn(fmt.Sprintf("Warning: empty project root in '%s'", distro))
		return
	}

	projectName := filepath.Base(projectRoot)
	wslProjectDest := "/home/admin/" + projectName
	wslProjectWindows := toWslPath(projectRoot)

	printStatus(m.ui, fmt.Sprintf("Syncing '%s' back to Windows...", distro))

	syncScript := fmt.Sprintf(
		`rsync -rlpt --info=progress2 --no-inc-recursive --delete --exclude='vendor/' --exclude='node_modules/' --exclude='.trellis/' %s/ %s/`,
		wslProjectDest, wslProjectWindows,
	)

	syncErr := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("wsl", []string{
		"-d", distro,
		"-u", "admin",
		"--", "bash", "-c", syncScript,
	}).Run()

	fmt.Print("\r\033[K")
	if syncErr != nil {
		m.ui.Warn(fmt.Sprintf("Warning: sync failed for '%s': %v", distro, syncErr))
	} else {
		printStatus(m.ui, fmt.Sprintf("%s '%s' synced to Windows", color.GreenString("[ok]"), distro))
	}
}

// DecodeWslOutput handles the UTF-16LE encoding that wsl.exe produces on
// Windows. Most command-line tools output UTF-8, but wsl.exe is a notable
// exception — it encodes list output as UTF-16LE, sometimes with a BOM
// (byte order mark: 0xFF 0xFE) prefix.
//
// This function detects UTF-16LE by looking for null bytes in the pattern
// typical of ASCII text encoded as UTF-16, then converts to a plain Go
// string (which is UTF-8).
func DecodeWslOutput(raw []byte) string {
	if len(raw) < 2 {
		return string(raw)
	}

	start := 0
	// Skip UTF-16LE BOM if present.
	if raw[0] == 0xFF && raw[1] == 0xFE {
		start = 2
	}

	// Heuristic: if the second byte of the first pair is 0x00, this is
	// likely UTF-16LE (ASCII characters are stored as [char, 0x00]).
	if start+1 < len(raw) && raw[start+1] == 0x00 {
		var buf []byte
		for i := start; i+1 < len(raw); i += 2 {
			if raw[i+1] == 0x00 {
				buf = append(buf, raw[i])
			}
		}
		return string(buf)
	}

	return string(raw[start:])
}

// isProvisioned checks whether a distro has been fully provisioned.
//
// Primary check: a marker file on the Windows filesystem written at the end
// of the Provision step. Fallback: the /etc/trellis-project-root breadcrumb
// inside the distro (written during bootstrap). This handles distros that
// were provisioned before the marker system existed, or whose marker file
// was lost. When the fallback succeeds, the marker is self-healed so future
// checks are fast.
func (m *Manager) isProvisioned(distro string) bool {
	markerPath := filepath.Join(m.ConfigPath, distro+".provisioned")
	if _, err := os.Stat(markerPath); err == nil {
		return true
	}

	// Fallback: check for the breadcrumb file inside the running distro.
	out, err := exec.Command("wsl", "-d", distro, "--", "cat", "/etc/trellis-project-root").Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		// Self-heal: write the marker so we don't shell into WSL every time.
		m.markProvisioned(distro)
		return true
	}

	return false
}

// IsProvisioned checks if the named instance has been fully provisioned.
// Used by vm_start.go to detect partially-created distros that need
// cleanup and re-creation.
func (m *Manager) IsProvisioned(name string) bool {
	return m.isProvisioned(distroName(name))
}

// markProvisioned writes the marker file that isProvisioned checks.
func (m *Manager) markProvisioned(distro string) {
	markerPath := filepath.Join(m.ConfigPath, distro+".provisioned")
	_ = os.MkdirAll(filepath.Dir(markerPath), 0755)
	_ = os.WriteFile(markerPath, []byte("ok\n"), 0644)
}

// ensureRootfs returns the path to a cached Ubuntu rootfs tarball,
// downloading it first if necessary.
func (m *Manager) ensureRootfs() (string, error) {
	tarball := filepath.Join(m.ConfigPath, "ubuntu-rootfs.tar.gz")

	// Already cached — nothing to do.
	if _, err := os.Stat(tarball); err == nil {
		return tarball, nil
	}

	ubuntuVersion := m.trellis.CliConfig.Vm.Ubuntu
	url, ok := UbuntuRootfsURLs[ubuntuVersion]
	if !ok {
		return "", fmt.Errorf(
			"no rootfs download URL for Ubuntu %s\n"+
				"Supported versions: %s\n"+
				"You can manually place a rootfs tarball at:\n  %s",
			ubuntuVersion, supportedUbuntuVersions(), tarball,
		)
	}

	printStatus(m.ui, fmt.Sprintf("Downloading Ubuntu %s rootfs...", ubuntuVersion))
	printStatus(m.ui, fmt.Sprintf("  URL: %s", url))

	if err := downloadFile(url, tarball); err != nil {
		return "", fmt.Errorf("could not download rootfs: %v\n"+
			"You can manually download the rootfs and place it at:\n  %s", err, tarball)
	}

	printStatus(m.ui, fmt.Sprintf("%s Download complete", color.GreenString("[ok]")))
	return tarball, nil
}

// downloadFile fetches a URL and writes it to dest atomically (via a temp file).
// Prints download progress to stdout.
func downloadFile(url string, dest string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Write to a temp file first so a partial download doesn't leave a
	// corrupt tarball that ensureRootfs would think is valid.
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		os.Remove(tmp) // clean up on failure; no-op if already renamed
	}()

	// Show download progress when Content-Length is available.
	totalBytes := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			written += int64(n)
			if totalBytes > 0 {
				pct := float64(written) / float64(totalBytes) * 100
				fmt.Printf("\r  %.0f%% (%d / %d MB)", pct, written/1024/1024, totalBytes/1024/1024)
			} else {
				fmt.Printf("\r  %d MB downloaded", written/1024/1024)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	fmt.Println()

	if err = f.Close(); err != nil {
		return err
	}

	return os.Rename(tmp, dest)
}

// writeInventory writes the Ansible inventory file.
//
// For WSL, Ansible runs inside the distro and provisions the local machine,
// so we use ansible_connection=local (no SSH needed).
//
// ansible_user=admin is required so Trellis's development override
// (web_user: "{{ ansible_user | default('web') }}") resolves to 'admin'.
// Without it, web_user defaults to 'web' and directories.yml sets web/
// to web:www-data — then Composer (running as admin) can't create web/wp/.
// Lima's inventory sets this the same way.
func (m *Manager) writeInventory() error {
	inventory := `default ansible_connection=local ansible_user=admin

[development]
default

[web]
default
`
	if err := os.WriteFile(m.InventoryPath(), []byte(inventory), 0644); err != nil {
		return fmt.Errorf("could not write inventory file: %v", err)
	}
	return nil
}

// toWslPath converts a Windows path (e.g. C:\Users\foo\bar) to a WSL
// mount path (e.g. /mnt/c/Users/foo/bar).
//
// WSL automatically mounts Windows drives under /mnt/<lowercase-letter>.
func toWslPath(windowsPath string) string {
	// Normalize to forward slashes.
	p := filepath.ToSlash(windowsPath)

	// Convert drive letter: "C:/..." → "/mnt/c/..."
	if len(p) >= 2 && p[1] == ':' {
		driveLetter := strings.ToLower(string(p[0]))
		p = "/mnt/" + driveLetter + p[2:]
	}

	return p
}

// BootstrapInstance installs Python, pip, and Ansible inside the WSL distro.
// This is called once after the distro is first created, before provisioning.
func (m *Manager) BootstrapInstance(name string) error {
	distro := distroName(name)

	printStatus(m.ui, "Bootstrapping WSL distro (installing Ansible)...")

	// Compute project paths. The project root (containing both trellis/ and
	// site/) is the parent of the trellis directory.
	projectRoot := filepath.Dir(m.trellis.Path)
	projectName := filepath.Base(projectRoot)
	wslProjectRoot := toWslPath(projectRoot)
	wslProjectDest := "/home/admin/" + projectName

	// Run apt-get update + install in a single shell command to minimize
	// the number of wsl.exe invocations. Then install Ansible via pip using
	// Trellis's requirements.txt. We read from the DrvFS source since the
	// ext4 copy hasn't happened yet at this point in the script.
	wslTrellisSrc := wslProjectRoot + "/trellis"

	bootstrapScript := `set -e
export DEBIAN_FRONTEND=noninteractive

# Prevent openssh-server's ssh.socket from starting during install.
# WSL2 pre-binds port 22 with its own SSH relay (kernel-level, no PID).
# Ubuntu 24.04's socket-activated SSH tries to bind the same port, causing
# deb-systemd-invoke to fail and leaving the package half-configured.
# The ssh.socket unit has ConditionPathExists=!/etc/ssh/sshd_not_to_be_run,
# so creating this file makes systemd skip the socket entirely.
# We don't need SSH anyway (ansible_connection=local).
mkdir -p /etc/ssh
touch /etc/ssh/sshd_not_to_be_run

apt-get update -qq
apt-get install -y -qq python3 python3-pip python3-venv rsync curl ca-certificates gnupg

# Install Node.js LTS (for Sage/frontend build tools like yarn dev).
# Unlike upstream Lima where Node runs on the host, WSL project files live
# on ext4 — so Node/yarn must be inside the distro where the developer works.
curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -
apt-get install -y -qq nodejs
corepack enable

pip3 install --break-system-packages --root-user-action=ignore -r ` + wslTrellisSrc + `/requirements.txt

# Create the web user and group that Trellis expects.
# On a real server these would be created by the 'users' role (server.yml),
# but dev.yml skips that role and assumes they exist.
getent group www-data >/dev/null 2>&1 || groupadd www-data
id -u web >/dev/null 2>&1 || useradd -m -N -g www-data -G www-data -s /bin/bash web
id -u admin >/dev/null 2>&1 || useradd -m -N -g admin -G sudo -s /bin/bash admin

# Give admin passwordless sudo so Ansible become: yes works.
echo 'admin ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/admin
chmod 440 /etc/sudoers.d/admin

# Configure WSL. Must come after user creation so admin user exists.
# - systemd=true: required for services (nginx, mariadb, journald, etc.)
# - default=admin: sets the default WSL user
# - metadata,umask=0022: enables Linux permission storage on NTFS.
#   Do NOT use fmask=0111 — it strips the execute bit from all DrvFS files,
#   which breaks VS Code's WSL extension (wslServer.sh: Permission denied).
# The distro must be restarted for these settings to take effect (handled below).
cat > /etc/wsl.conf << 'WSLCONF'
[boot]
systemd=true

[user]
default=admin

[automount]
options = "metadata,umask=0022"
WSLCONF

# Create .ssh directory for admin user (needed by Ansible known_hosts module).
mkdir -p /home/admin/.ssh
chmod 700 /home/admin/.ssh
chown admin:admin /home/admin/.ssh
`

	// Copy the ENTIRE project (trellis/ + site/ + .git/) from Windows into
	// WSL's native ext4 filesystem. This is the critical step that gives us:
	//   1. Fast PHP I/O (ext4 vs DrvFS/9p = 77ms vs 14s page loads)
	//   2. Intact git repo (developers use VS Code + WSL git as normal)
	//   3. Single workspace (trellis/ + site/ together, natural project layout)
	//
	// The copy goes through 9p (slow) but is a ONE-TIME cost during initial
	// setup. After this, the developer works entirely within the WSL distro
	// using VS Code's WSL extension.
	bootstrapScript += fmt.Sprintf(
		"echo 'Copying project files to WSL filesystem...'\nmkdir -p %s && rsync -rlpt --chmod=D755,F644 --info=progress2 %s/ %s/\n",
		wslProjectDest, wslProjectRoot, wslProjectDest,
	)
	bootstrapScript += fmt.Sprintf(
		"chown -R admin:admin %s\n",
		wslProjectDest,
	)

	// Write the Windows project root path inside the distro so that
	// stopOtherDistros can SyncBack without needing the trellis project loaded.
	bootstrapScript += fmt.Sprintf(
		"echo '%s' > /etc/trellis-project-root\n",
		projectRoot,
	)

	// Strip the execute bit from .vault_pass in the project copy.
	// DrvFS metadata marks all files executable; Ansible interprets an
	// executable .vault_pass as a script and tries to run it, which fails
	// with "Exec format error" since it's a plain text file.
	bootstrapScript += fmt.Sprintf(
		"chmod 644 %s/trellis/.vault_pass 2>/dev/null || true\n",
		wslProjectDest,
	)

	// Copy vault password file to a secure location inside the distro.
	// Even though trellis/.vault_pass is now on ext4, Ansible may complain
	// about permissions depending on the umask. The dedicated copy is safer.
	bootstrapScript += "mkdir -p /home/admin/.trellis\n"
	bootstrapScript += fmt.Sprintf(
		"cp %s/trellis/.vault_pass /home/admin/.trellis/.vault_pass\n",
		wslProjectDest,
	)
	bootstrapScript += "chmod 600 /home/admin/.trellis/.vault_pass\n"
	bootstrapScript += "chown -R admin:admin /home/admin/.trellis\n"

	// Install the trellis CLI binary inside the distro so developers can
	// run `trellis provision development`, `trellis db open`, etc. from
	// the VS Code WSL terminal.
	//
	// For fork/dev builds: look for a cross-compiled `trellis-linux` binary
	// next to the running executable and copy it in.
	// For upstream releases: this would use the official install script instead.
	exePath, _ := os.Executable()
	linuxBinary := filepath.Join(filepath.Dir(exePath), "trellis-linux")
	if _, err := os.Stat(linuxBinary); err == nil {
		wslLinuxBinary := toWslPath(linuxBinary)
		bootstrapScript += fmt.Sprintf(
			"cp %s /usr/local/bin/trellis && chmod 755 /usr/local/bin/trellis\n",
			wslLinuxBinary,
		)
	}

	// Bind-mount each site's directory from the ext4 project copy to the
	// /srv/www/ path that nginx expects. Bind mounts (not symlinks) keep
	// $realpath_root within /srv/www/, satisfying PHP-FPM's open_basedir.
	for siteName, site := range m.Sites {
		siteRelPath := site.LocalPath // e.g. "../site"
		// Resolve relative path: trellis/../site → site
		siteDirName := filepath.Base(filepath.Join("trellis", siteRelPath))

		bootstrapScript += fmt.Sprintf(
			"mkdir -p /srv/www/%s/current\n",
			siteName,
		)
		bootstrapScript += fmt.Sprintf(
			"mount --bind %s/%s /srv/www/%s/current\n",
			wslProjectDest, siteDirName, siteName,
		)
		// Add fstab entry so the bind mount survives WSL restarts.
		bootstrapScript += fmt.Sprintf(
			"grep -q '/srv/www/%s/current' /etc/fstab || echo '%s/%s /srv/www/%s/current none bind,nofail 0 0' >> /etc/fstab\n",
			siteName, wslProjectDest, siteDirName, siteName,
		)
	}

	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("wsl", []string{"-d", distro, "--", "bash", "-c", bootstrapScript}).Run()

	if err != nil {
		return fmt.Errorf("could not bootstrap WSL distro: %v", err)
	}

	// Restart the distro so /etc/wsl.conf metadata mount option takes effect.
	// Without this, chmod/fchmod on /mnt/c/ files will still fail.
	printStatus(m.ui, "Restarting WSL distro to apply mount options...")
	_ = command.Cmd("wsl", []string{"--terminate", distro}).Run()
	err = command.Cmd("wsl", []string{"-d", distro, "--", "/bin/true"}).Run()
	if err != nil {
		return fmt.Errorf("could not restart WSL distro: %v", err)
	}

	// Re-establish the keepalive process. The --terminate above killed the
	// original `sleep infinity` started by StartInstance.
	keepalive := exec.Command("wsl", "-d", distro, "--exec", "sleep", "infinity")
	if err := keepalive.Start(); err != nil {
		m.ui.Warn(fmt.Sprintf("Warning: could not start keepalive: %v", err))
	} else {
		go func() { _ = keepalive.Wait() }()
	}

	printStatus(m.ui, fmt.Sprintf("%s Ansible installed", color.GreenString("[ok]")))
	return nil
}

// Provision runs ansible-galaxy install and ansible-playbook inside the WSL
// distro. This replaces the host-side ProvisionCommand for WSL.
//
// The trellis directory lives on WSL ext4 at /home/admin/<project>/trellis/
// (copied during bootstrap). This is much faster than reading from /mnt/c/.
func (m *Manager) Provision(name string) error {
	distro := distroName(name)

	// Use the ext4 copy of the trellis directory for provisioning.
	projectRoot := filepath.Dir(m.trellis.Path)
	projectName := filepath.Base(projectRoot)
	trellisDir := "/home/admin/" + projectName + "/trellis"
	inventoryPath := toWslPath(m.InventoryPath())
	ansibleCfg := trellisDir + "/ansible.cfg"

	// Set ANSIBLE_CONFIG explicitly. Even though the ext4 copy has proper
	// permissions, this is consistent with our bootstrap approach.
	envPrefix := "export ANSIBLE_CONFIG=" + ansibleCfg + " ANSIBLE_HOST_KEY_CHECKING=False ANSIBLE_VAULT_PASSWORD_FILE=/home/admin/.trellis/.vault_pass && "

	// Install Galaxy roles inside WSL
	printStatus(m.ui, "Installing Ansible Galaxy roles...")

	galaxyFiles := []string{"galaxy.yml", "requirements.yml"}
	for _, f := range galaxyFiles {
		fullPath := filepath.Join(m.trellis.Path, f)
		if _, err := os.Stat(fullPath); err == nil {
			err := command.WithOptions(
				command.WithTermOutput(),
			).Cmd("wsl", []string{
				"-d", distro,
				"-u", "admin",
				"--cd", trellisDir,
				"--", "bash", "-c", envPrefix + "ansible-galaxy install -r " + f,
			}).Run()

			if err != nil {
				m.ui.Warn(fmt.Sprintf("Warning: ansible-galaxy install failed: %v", err))
			}
			break
		}
	}

	// Run ansible-playbook dev.yml
	printStatus(m.ui, "Running Ansible provisioning...")

	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("wsl", []string{
		"-d", distro,
		"-u", "admin",
		"--cd", trellisDir,
		"--", "bash", "-c", envPrefix + "ansible-playbook dev.yml --inventory=" + inventoryPath + " -e env=development",
	}).Run()

	if err != nil {
		return fmt.Errorf("provisioning failed: %v", err)
	}

	// Tune opcache for WSL. Even though files are now on ext4 (not DrvFS),
	// a small revalidate delay reduces unnecessary stat() calls during
	// development. 2 seconds is development-friendly (max 2s stale window).
	printStatus(m.ui, "Tuning opcache for WSL performance...")
	opcacheScript := `set -e
PHP_VER=$(php -r 'echo PHP_MAJOR_VERSION."_".PHP_MINOR_VERSION;' | tr '_' '.')
printf '[opcache]\nopcache.revalidate_freq=2\n' > /etc/php/${PHP_VER}/fpm/conf.d/99-wsl-performance.ini
systemctl restart php${PHP_VER}-fpm
`
	opcacheScriptPath := filepath.Join(m.ConfigPath, "opcache-tune.sh")
	if err := os.WriteFile(opcacheScriptPath, []byte(opcacheScript), 0644); err != nil {
		m.ui.Warn(fmt.Sprintf("Warning: could not write opcache script: %v", err))
	} else {
		_ = command.WithOptions(
			command.WithTermOutput(),
		).Cmd("wsl", []string{
			"-d", distro,
			"-u", "root",
			"--", "bash", toWslPath(opcacheScriptPath),
		}).Run()
	}

	// Import self-signed SSL certs into the Windows Trusted Root CA store
	// so browsers accept https://<site>.test without warnings.
	if err := m.TrustSslCerts(distro); err != nil {
		m.ui.Warn(fmt.Sprintf("Warning: could not trust SSL certs: %v", err))
	}

	// Mark the distro as fully provisioned. StartInstance checks this to
	// detect partially-created distros (e.g. cancelled during bootstrap).
	m.markProvisioned(distro)

	return nil
}

// TrustSslCerts extracts self-signed SSL certificates from the WSL distro
// and imports them into the Windows Trusted Root Certification Authorities
// store. This eliminates browser warnings for https://*.test sites.
//
// Only processes sites that have ssl.enabled: true in wordpress_sites.yml.
// Uses certutil.exe via UAC elevation (same pattern as hosts file updates).
func (m *Manager) TrustSslCerts(distro string) error {
	var sslSites []string
	for siteName, site := range m.Sites {
		if site.SslEnabled() {
			sslSites = append(sslSites, siteName)
		}
	}

	if len(sslSites) == 0 {
		m.ui.Warn("No SSL-enabled sites found in development config. Set ssl.enabled: true in wordpress_sites.yml.")
		return nil
	}

	printStatus(m.ui, "Importing SSL certificates into Windows trust store...")

	certDir := filepath.Join(m.ConfigPath, "certs")
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("could not create cert directory: %v", err)
	}

	var certPaths []string
	for _, siteName := range sslSites {
		// Trellis stores certs at /etc/nginx/ssl/<siteName>.cert
		remoteCert := fmt.Sprintf("/etc/nginx/ssl/%s.cert", siteName)
		localCert := filepath.Join(certDir, siteName+".crt")

		// Extract the cert from the distro.
		output, err := command.Cmd("wsl", []string{
			"-d", distro, "-u", "root", "--", "cat", remoteCert,
		}).Output()

		if err != nil {
			m.ui.Warn(fmt.Sprintf("Warning: could not read cert for %s: %v", siteName, err))
			continue
		}

		if err := os.WriteFile(localCert, output, 0644); err != nil {
			m.ui.Warn(fmt.Sprintf("Warning: could not save cert for %s: %v", siteName, err))
			continue
		}

		certPaths = append(certPaths, localCert)
	}

	if len(certPaths) == 0 {
		return nil
	}

	// Build a PowerShell script that imports all certs in one UAC prompt.
	var importCmds []string
	for _, certPath := range certPaths {
		importCmds = append(importCmds,
			fmt.Sprintf(`certutil -addstore Root \"%s\"`, certPath),
		)
	}

	script := strings.Join(importCmds, "; ")

	printStatus(m.ui, "Admin privileges required to trust certificates -- a UAC prompt will appear.")

	if err := command.Cmd("powershell", []string{
		"-Command",
		fmt.Sprintf(
			"Start-Process powershell.exe -Verb RunAs -Wait -ArgumentList '-NoProfile','-Command','%s'",
			script,
		),
	}).Run(); err != nil {
		return err
	}

	printStatus(m.ui, fmt.Sprintf("SSL certificates trusted for %d site(s).", len(certPaths)))
	return nil
}

// syncConfigFromWSL rsyncs trellis/group_vars/ from the WSL ext4 project
// back to the Windows filesystem. This is called in NewManager when the
// distro is running, so that any config changes made inside WSL (e.g.
// enabling SSL, adding site hosts) are visible to Windows-side commands.
//
// Only syncs group_vars/ (not the full project) to keep it fast — this
// directory contains wordpress_sites.yml and vault.yml which drive most
// command behavior. A full project sync happens in SyncBack/vm stop.
func (m *Manager) syncConfigFromWSL(distro string) {
	projectRoot := filepath.Dir(m.trellis.Path)
	projectName := filepath.Base(projectRoot)
	wslProjectDest := "/home/admin/" + projectName
	wslProjectWindows := toWslPath(projectRoot)

	syncScript := fmt.Sprintf(
		`rsync -rlpt %s/trellis/group_vars/ %s/trellis/group_vars/`,
		wslProjectDest, wslProjectWindows,
	)

	// Best-effort: if rsync fails (distro not ready, rsync missing),
	// continue with the existing Windows-side config.
	_ = command.Cmd("wsl", []string{
		"-d", distro,
		"-u", "admin",
		"--", "bash", "-c", syncScript,
	}).Run()
}

// SyncToWSL copies the trellis/ directory from Windows into the WSL ext4
// project. This is used before re-provisioning so that any config changes
// the developer made on the Windows side are reflected inside WSL.
//
// Only syncs trellis/ (not site/) since site files are edited inside WSL
// via VS Code's WSL extension. Trellis config is the exception because
// developers may edit it from either side.
func (m *Manager) SyncToWSL(name string) error {
	distro := distroName(name)

	if !m.distroExists(distro) {
		return fmt.Errorf("WSL distro does not exist. Run `trellis vm start` first.")
	}

	projectRoot := filepath.Dir(m.trellis.Path)
	projectName := filepath.Base(projectRoot)
	wslProjectRoot := toWslPath(projectRoot)
	wslProjectDest := "/home/admin/" + projectName

	printStatus(m.ui, "Syncing trellis/ config to WSL...")

	syncScript := fmt.Sprintf(
		`rsync -rlpt --chmod=D755,F644 --delete %s/trellis/ %s/trellis/`,
		wslProjectRoot, wslProjectDest,
	)

	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("wsl", []string{
		"-d", distro,
		"-u", "admin",
		"--", "bash", "-c", syncScript,
	}).Run()

	if err != nil {
		return fmt.Errorf("sync to WSL failed: %v", err)
	}

	printStatus(m.ui, fmt.Sprintf("%s Trellis config synced", color.GreenString("[ok]")))
	return nil
}

// Reprovision syncs config changes from Windows, then runs provisioning.
// This is the WSL equivalent of `trellis provision development`.
func (m *Manager) Reprovision(name string) error {
	if err := m.SyncToWSL(name); err != nil {
		return err
	}

	return m.Provision(name)
}

// SyncBack copies changed files from the WSL ext4 project back to the
// Windows filesystem. This keeps the Windows-side repo up to date so
// GitHub Desktop and other Windows tools can see the latest changes.
//
// Uses rsync for efficient incremental sync — only changed files are
// transferred through 9p, making subsequent syncs fast (seconds, not minutes).
//
// Direction: WSL ext4 → Windows (one-way). Never the reverse during sync.
func (m *Manager) SyncBack(name string) error {
	distro := distroName(name)

	if !m.distroExists(distro) {
		return fmt.Errorf("WSL distro does not exist. Run `trellis vm start` first.")
	}

	if !m.distroRunning(distro) {
		return fmt.Errorf("WSL distro is not running. Start it with `trellis vm start` first.")
	}

	projectRoot := filepath.Dir(m.trellis.Path)
	projectName := filepath.Base(projectRoot)
	wslProjectDest := "/home/admin/" + projectName
	wslProjectWindows := toWslPath(projectRoot)

	printStatus(m.ui, "Syncing project files from WSL to Windows...")

	// rsync flags:
	//   -rlpt: recursive, links, perms, times (like -a but without group/owner
	//          which fail on DrvFS with "Operation not permitted")
	//   --delete: remove files on Windows side that were deleted in WSL
	//   --exclude: skip directories that are large, generated, or WSL-specific
	//   --no-inc-recursive: scan all files first so progress % is accurate
	//   Trailing slashes on source ensure contents are synced, not the dir itself.
	syncScript := fmt.Sprintf(
		`rsync -rlpt --info=progress2 --no-inc-recursive --delete --exclude='vendor/' --exclude='node_modules/' --exclude='.trellis/' %s/ %s/`,
		wslProjectDest, wslProjectWindows,
	)

	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("wsl", []string{
		"-d", distro,
		"-u", "admin",
		"--", "bash", "-c", syncScript,
	}).Run()

	if err != nil {
		return fmt.Errorf("sync failed: %v", err)
	}

	// Clear the rsync progress line before printing the final status.
	fmt.Print("\r\033[K")
	printStatus(m.ui, fmt.Sprintf("%s Project synced to Windows", color.GreenString("[ok]")))
	return nil
}
