#!/bin/bash
# Dumps a server's effective configuration, as reported by the tools
# themselves, in "=== section" blocks. Run as root. Read-only and best
# effort: missing tools just produce empty sections.
set -u
export LC_ALL=C PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
sec() { printf '\n=== %s\n' "$1"; }

php_v=$(ls /usr/sbin/php-fpm* 2>/dev/null | sed -n 's#.*php-fpm\([0-9.]*\)$#\1#p' | sort -V | tail -1)
# Accounts that can log in (system UIDs included: provisioners differ).
people=$(getent passwd | awk -F: '$7 !~ /(nologin|false|sync|halt|shutdown)$/ {print $1}' | grep -v '\.linux$')

sec hostname; hostname
sec os; . /etc/os-release; echo "$PRETTY_NAME $(uname -m)"
sec packages; dpkg-query -W -f='${db:Status-Abbrev}\t${Package}\t${Version}\n' | awk -F'\t' '$1 ~ /^ii/ {print $2"\t"$3}'
sec apt-sources; apt-get indextargets --format '$(SITE) $(RELEASE) $(COMPONENT)' 2>/dev/null | sort -u
sec units-enabled; systemctl list-unit-files --no-legend --state=enabled,masked 2>/dev/null | awk '{print $1"\t"$2}'
sec units-active; systemctl list-units --no-legend --plain --state=active --type=service,socket,timer 2>/dev/null | awk '{print $1}'
sec listen; ss -Htlnup 2>/dev/null | awk '{print $1"\t"$5"\t"$7}'
sec users
for u in $people; do
  getent passwd "$u" | awk -F: -v g="$(id -nG "$u" | tr ' ' '\n' | sort | tr '\n' ',')" '{print $1"\thome="$6" shell="$7" groups="g}'
done
sec sudo
for u in $people; do sudo -l -U "$u" 2>/dev/null | sed -n '/may run the following/,$p' | tail -n +2 | sed "s/^ *//; s/^/$u\t/"; done
sec authorized-keys
for u in $people; do
  h=$(getent passwd "$u" | cut -d: -f6)
  [ -f "$h/.ssh/authorized_keys" ] && ssh-keygen -lf "$h/.ssh/authorized_keys" 2>/dev/null | awk -v u="$u" '{print u"\t"$2" "$NF}'
done
sec sshd; sshd -T 2>/dev/null
sec nginx; nginx -T 2>/dev/null
if [ -n "$php_v" ]; then
  sec php-fpm-pools; "php-fpm$php_v" -tt 2>&1
  sec php-ini-fpm; PHP_INI_SCAN_DIR=/etc/php/$php_v/fpm/conf.d php -c "/etc/php/$php_v/fpm/php.ini" -r 'foreach (ini_get_all(null, false) as $k => $v) echo "$k\t$v\n";' 2>/dev/null
  sec php-ini-cli; php -r 'foreach (ini_get_all(null, false) as $k => $v) echo "$k\t$v\n";' 2>/dev/null
  sec php-modules-fpm; PHP_INI_SCAN_DIR=/etc/php/$php_v/fpm/conf.d php -c "/etc/php/$php_v/fpm/php.ini" -m 2>/dev/null
fi
if command -v mariadb >/dev/null; then
  sec mariadb-vars; mariadb -NBe 'SHOW GLOBAL VARIABLES' 2>/dev/null
  sec mariadb-dbs; mariadb -NBe 'SHOW DATABASES' 2>/dev/null
  sec mariadb-grants
  mariadb -NBe "SELECT CONCAT(QUOTE(user),'@',QUOTE(host)) FROM mysql.user" 2>/dev/null | while read -r ident; do
    mariadb -NBe "SHOW GRANTS FOR $ident" 2>/dev/null | sed "s/^/$ident\t/"
  done
fi
if command -v fail2ban-client >/dev/null; then
  sec fail2ban
  for j in $(fail2ban-client status 2>/dev/null | sed -n 's/.*Jail list:[[:space:]]*//p' | tr -d ','); do
    for k in maxretry findtime bantime logpath actions; do
      printf '%s\t%s\t%s\n' "$j" "$k" "$(fail2ban-client get "$j" "$k" 2>/dev/null | tr '\n' ' ' | sed 's/  */ /g')"
    done
  done
fi
sec firewall; nft -j list ruleset 2>/dev/null
# ferm (stock Trellis) writes legacy iptables rules, which nft can't see.
sec firewall-legacy; echo '# family ip'; iptables-legacy-save 2>/dev/null; echo '# family ip6'; ip6tables-legacy-save 2>/dev/null
sec sysctl; sysctl -a 2>/dev/null
sec timedate; timedatectl show 2>/dev/null; for s in chrony systemd-timesyncd ntp ntpsec; do echo "service.$s=$(systemctl is-active $s 2>/dev/null)"; done
sec journald; systemd-analyze cat-config systemd/journald.conf 2>/dev/null
sec swap; swapon --show=NAME,SIZE --noheadings 2>/dev/null
sec timers; systemctl list-units --no-legend --plain --all --type=timer 2>/dev/null | awk '{print $1}'
sec cron
for u in root $people; do crontab -l -u "$u" 2>/dev/null | grep -v '^#' | sed "s/^/$u\t/"; done
ls /etc/cron.d 2>/dev/null | sed 's/^/cron.d\t/'
sec tree; find /srv /var/log/nginx $(for u in $people; do getent passwd "$u" | cut -d: -f6; done | sort -u) -xdev -maxdepth 4 \
  -not -path '*/.cache*' -printf '%p\t%y %u:%g %m\n' 2>/dev/null
sec logrotate; logrotate -d /etc/logrotate.conf 2>&1 | grep -A1 '^rotating pattern' | grep -v '^--$' | paste - -
sec hosts; grep -v '^#' /etc/hosts | grep -v '^[[:space:]]*$'
sec msmtp; grep -v -e '^#' -e '^[[:space:]]*$' -e password /etc/msmtprc 2>/dev/null
sec tools
echo "wp=$(wp --allow-root --version 2>/dev/null)"
echo "composer=$(COMPOSER_ALLOW_SUPERUSER=1 composer --version 2>/dev/null | awk '{print $1, $2, $3}')"
sec http
for host in example.com www.example.com unknown.invalid; do
  for scheme in http https; do
    for path in / /wp-login.php /xmlrpc.php /.env /composer.json /readme.html /wp-content/uploads/x.php /app/uploads/x.php; do
      out=$(curl -sk --max-time 5 -o /dev/null -D - --resolve "$host:80:127.0.0.1" --resolve "$host:443:127.0.0.1" "$scheme://$host$path" 2>/dev/null | tr -d '\r')
      status=$(printf '%s\n' "$out" | head -1 | awk '{print $2}')
      printf '%s\t%s\n' "$scheme://$host$path" "status=${status:-none}"
      printf '%s\n' "$out" | tail -n +2 | grep -iv -e '^date:' -e '^content-length:' -e '^etag:' -e '^last-modified:' -e '^$' |
        sed "s#^#$scheme://$host$path\theader #"
    done
  done
done
sec reboot-required; [ -f /var/run/reboot-required ] && echo yes || echo no
