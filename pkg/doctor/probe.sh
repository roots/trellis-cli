#!/bin/sh
# trellis doctor probe
#
# Collects read-only facts about a Trellis server without sudo and prints them
# as key=value lines between begin/end markers. Anything else printed (e.g. by
# a login shell) is ignored by trellis-cli.
#
# Usage: probe.sh PHP_VERSION NGINX_PACKAGE MARIADB_SERVER_PACKAGE
# Any argument may be empty.

export LC_ALL=C

php_version=$1
nginx_package=$2
mariadb_package=$3

kv() {
  printf '%s=%s\n' "$1" "$2"
}

mtime() {
  stat -c %Y "$1" 2>/dev/null
}

policy() {
  pkg=$1

  if ! out=$(apt-cache policy "$pkg" 2>/dev/null); then
    # apt itself is broken (e.g. a malformed source); report what dpkg knows.
    kv "pkg.$pkg.error" 1
    kv "pkg.$pkg.installed" "$(dpkg-query -W -f '${db:Status-Status} ${Version}' "$pkg" 2>/dev/null | awk '$1 == "installed" { print $2 }')"
    return 0
  fi

  # Unknown to apt: not installed and not in any source.
  [ -n "$out" ] || return 0

  installed=$(printf '%s\n' "$out" | awk '/^  Installed:/ { print $2; exit }')
  candidate=$(printf '%s\n' "$out" | awk '/^  Candidate:/ { print $2; exit }')
  # Origin of the candidate version: the first source line under it in the
  # version table that isn't dpkg's own status file. Version lines are
  # indented 5 spaces (or " *** "); source lines 7+ (width varies with the
  # pin priority). When an installed package's source is gone, apt reports
  # the installed version as the candidate with only /var/lib/dpkg/status.
  origin=$(printf '%s\n' "$out" | awk -v c="$candidate" '
    /^ \*\*\* / { v = $2; next }
    /^     [^ ]/ { v = $1; next }
    /^      +-?[0-9]+ / { if (v == c && o == "" && NF >= 2 && $2 !~ /^\/var\/lib\/dpkg/) o = $2 " " $3 }
    END { print o }')

  kv "pkg.$pkg.installed" "$installed"
  kv "pkg.$pkg.candidate" "$candidate"
  kv "pkg.$pkg.origin" "$origin"

  case "$installed" in ''|'(none)') return 0 ;; esac
  case "$candidate" in ''|'(none)') return 0 ;; esac

  if dpkg --compare-versions "$installed" lt "$candidate"; then
    kv "pkg.$pkg.cmp" lt
  elif dpkg --compare-versions "$installed" gt "$candidate"; then
    kv "pkg.$pkg.cmp" gt
  elif dpkg --compare-versions "$installed" eq "$candidate"; then
    kv "pkg.$pkg.cmp" eq
  fi

  if printf '%s\n' "$held" | grep -qxF -- "$pkg"; then
    kv "pkg.$pkg.held" 1
  fi
}

echo "doctor_begin v=1"

kv now "$(date +%s)"

if [ -r /etc/os-release ]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  kv os.id "$ID"
  kv os.version_id "$VERSION_ID"
  kv os.codename "$VERSION_CODENAME"
  kv os.name "$PRETTY_NAME"

  if [ -r /usr/share/distro-info/ubuntu.csv ] && [ -n "$VERSION_CODENAME" ]; then
    kv os.eol "$(awk -F, -v s="$VERSION_CODENAME" '
      NR == 1 { for (i = 1; i <= NF; i++) if ($i == "eol") col = i; next }
      col && $3 == s { print $col; exit }' /usr/share/distro-info/ubuntu.csv)"
  fi
fi

running=$(uname -r)
kv kernel.running "$running"
flavor=${running##*-}
# Kernel images are root-only (0600) but their names are listable.
kv kernel.latest "$(for k in /boot/vmlinuz-*-"$flavor"; do [ -e "$k" ] && printf '%s\n' "${k#/boot/vmlinuz-}"; done | sort -V | tail -n 1)"
kv uptime "$(cut -d. -f1 /proc/uptime 2>/dev/null)"

if [ -e /var/run/reboot-required ]; then
  kv reboot.required 1
else
  kv reboot.required 0
fi
if [ -r /var/run/reboot-required.pkgs ]; then
  kv reboot.pkgs "$(sort -u /var/run/reboot-required.pkgs | paste -sd, -)"
fi
if [ -x /usr/share/update-notifier/notify-reboot-required ]; then
  kv reboot.notifier 1
else
  kv reboot.notifier 0
fi

kv apt.update_success "$(mtime /var/lib/apt/periodic/update-success-stamp)"
kv apt.pkgcache "$(mtime /var/cache/apt/pkgcache.bin)"

if [ -x /usr/lib/update-notifier/apt-check ]; then
  # Prints "updates;security" to stderr.
  kv apt.check "$(/usr/lib/update-notifier/apt-check 2>&1 >/dev/null | tail -n 1)"
else
  if upgradable=$(apt list --upgradable 2>/dev/null); then
    kv apt.upgradable_security "$(printf '%s\n' "$upgradable" | grep -c -- '-security')"
  fi
fi

if [ "$(dpkg-query -W -f '${db:Status-Status}' unattended-upgrades 2>/dev/null)" = installed ]; then
  kv uu.installed 1
else
  kv uu.installed 0
fi
UU=
eval "$(apt-config shell UU APT::Periodic::Unattended-Upgrade 2>/dev/null)"
kv uu.setting "$UU"
kv uu.timer "$(systemctl is-enabled apt-daily-upgrade.timer 2>/dev/null)"
kv uu.stamp "$(mtime /var/lib/apt/periodic/unattended-upgrades-stamp)"

held=$(apt-mark showhold 2>/dev/null)

php_packages=
# Status-Status is "installed" for held packages too (Status-Abbrev is "hi").
for pkg in $(dpkg-query -W -f '${db:Status-Status} ${Package}\n' 'php*-fpm' 2>/dev/null | awk '$1 == "installed" { print $2 }'); do
  branch=${pkg#php}
  branch=${branch%-fpm}
  [ -n "$branch" ] || continue
  kv "php.fpm.$branch" "$(systemctl is-active "$pkg" 2>/dev/null)"
  php_packages="$php_packages $pkg"
done

if [ -n "$php_version" ]; then
  case " $php_packages " in
    *" php$php_version-fpm "*) ;;
    *) php_packages="$php_packages php$php_version-fpm" ;;
  esac
fi

for pkg in $nginx_package $mariadb_package $php_packages; do
  policy "$pkg"
done

echo "doctor_end"
