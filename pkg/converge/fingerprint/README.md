# fingerprint: compare servers by effective configuration

`converge` should leave a server in the same state as a stock Trellis
(Ansible) provision, apart from differences we choose. Files can't be
compared directly (different layouts, `ferm` vs `nftables`), and behaviour
tests only cover what someone thought to test. Instead, `probe.sh` asks each
tool for its *resolved* configuration:

| Area | Source |
|---|---|
| nginx | `nginx -T`, includes inlined, keyed by server (listen + names) and location |
| sshd | `sshd -T` |
| php | `php-fpm -tt` (pools), effective ini for FPM and CLI, loaded modules |
| mariadb | `SHOW GLOBAL VARIABLES`, databases, grants (passwords masked) |
| firewall | `nft -j list ruleset` reduced to policy, accepted ports, rate limits |
| fail2ban | per-jail maxretry/findtime/bantime/logpath/actions |
| system | packages, apt sources, enabled/active units, listening sockets, users, sudo rules, authorized key fingerprints, sysctl, timezone, journald, swap, timers, cron, logrotate, hosts, msmtp |
| layout | owner/mode tree of `/srv`, `/var/log/nginx` and user homes |
| http | status and headers for a matrix of hosts × paths × schemes |

`Parse` normalizes the output into sorted `area/key → value` facts.
`Compare` takes four probes, each server **before** and **after**
provisioning, and keeps only facts a provisioner changed. Each one is then
classed as same, differ, only Trellis, or only converge. Differences we
intend go in `compare.yml` with a reason, so the report shows what's left.

To build both servers and compare them in one go (about 10 minutes):

```sh
pkg/converge/fingerprint/compare-lima.sh ~/dev/trellis
```

This creates two fresh Lima VMs. It provisions one with your Trellis checkout's stock production config (via Ansible) and the other with an equivalent `trellis.yml` (via converge), then writes `report.md`.

Running the tool by hand:

```sh
go build -o fingerprint ./pkg/converge/fingerprint/cmd/fingerprint
./fingerprint probe | limactl shell vm sudo bash -s > vm.before.txt
# provision...
./fingerprint compare -config pkg/converge/fingerprint/compare.yml \
  trellis.before.txt trellis.after.txt converge.before.txt converge.after.txt
```
