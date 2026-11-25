#!/usr/bin/env bash
# Provisions two fresh Ubuntu 24.04 Lima VMs, one with stock Trellis
# (Ansible) and one with `trellis converge`, using equivalent config, and
# compares their effective configuration (see README.md).
#
#   pkg/converge/fingerprint/compare-lima.sh ~/dev/trellis [workdir]
#
# Needs: go, limactl, and a Trellis checkout with its virtualenv
# (`trellis init`). The VMs are named trellis-cmp-ansible and
# trellis-cmp-converge and are left running for inspection
# (`limactl shell trellis-cmp-converge`); delete them with
# `limactl delete -f trellis-cmp-ansible trellis-cmp-converge`.
set -euo pipefail

[ $# -ge 1 ] || { sed -n '2,13p' "$0"; exit 2; }
TRELLIS=$(cd "$1" && pwd)
WORK=${2:-$(mktemp -d "${TMPDIR:-/tmp}/trellis-compare.XXXXXX")}
mkdir -p "$WORK"
WORK=$(cd "$WORK" && pwd)
CLI=$(cd "$(dirname "$0")/../../.." && pwd)
KEY=$HOME/.lima/_config/user
VENV=$TRELLIS/.trellis/virtualenv/bin
A=trellis-cmp-ansible
C=trellis-cmp-converge

for cmd in go limactl; do command -v $cmd >/dev/null || { echo "missing $cmd" >&2; exit 1; }; done
[ -x "$VENV/ansible-playbook" ] || { echo "no Ansible in $VENV (run \`trellis init\` in $TRELLIS)" >&2; exit 1; }
log() { printf '\n==> %s\n' "$*"; }

log "Building trellis (with embedded agents) and fingerprint into $WORK"
(cd "$CLI" && go generate ./pkg/converge/agentbin >/dev/null &&
  go build -o "$WORK/trellis" . && go build -o "$WORK/fingerprint" ./pkg/converge/fingerprint/cmd/fingerprint)
"$WORK/fingerprint" probe >"$WORK/probe.sh"

vm() {
  limactl delete -f "$1" >/dev/null 2>&1 || true
  limactl create --name="$1" --cpus=2 --memory=3 --disk=20 --tty=false template://ubuntu-24.04 >/dev/null 2>&1
  limactl start "$1" --tty=false >/dev/null 2>&1
  # Like a fresh cloud server: root login with your key.
  limactl shell "$1" sh -c 'sudo install -d -m 700 /root/.ssh && sudo install -m 600 ~/.ssh/authorized_keys /root/.ssh/authorized_keys'
}
probe() { limactl shell "$1" sudo bash -s <"$WORK/probe.sh" >"$WORK/$1.$2.txt" 2>/dev/null; }
port() { limactl list "$1" --format '{{.SSHLocalPort}}'; }

log "Creating VMs $A and $C"
vm $A & vm $C & wait
probe $A before & probe $C before & wait

log "Preparing stock Trellis (copy of $TRELLIS)"
rsync -a --delete --exclude .git --exclude .trellis "$TRELLIS/" "$WORK/trellis/"
(cd "$WORK/trellis" && "$VENV/ansible-galaxy" install -r galaxy.yml -p vendor/roles --force >/dev/null)
line="cmp ansible_host=127.0.0.1 ansible_port=$(port $A) ansible_ssh_private_key_file=$KEY ansible_ssh_common_args='-o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no'"
printf '[production]\n%s\n\n[web]\n%s\n' "$line" "$line" >"$WORK/trellis/hosts/production"
sed -i.orig -e "s#lookup('file', '~/.ssh/id_rsa.pub', errors='ignore')#lookup('file', '$KEY.pub')#" \
  -e "s#lookup('file', '~/.ssh/id_ed25519.pub', errors='ignore')#lookup('file', '$KEY.pub')#" "$WORK/trellis/group_vars/all/users.yml"

log "Preparing the equivalent converge project"
P=$WORK/converge
mkdir -p "$P/keys"
cp "$KEY.pub" "$P/keys/admin.pub"
cat >"$P/trellis.yml" <<EOF
version: 1
# Mirrors stock Trellis production defaults (group_vars/all + production).
defaults:
  timezone: Etc/UTC
  php: { version: "8.4" }
  mail:
    smtp: smtp.example.com:587
    user: smtp_user
    password: secret:mail_password
    from: admin@example.com
sites:
  example.com:
    repo: git@github.com:example/example.com.git
    path: site
access:
  admins: [file:keys/admin.pub]
  deploy: [file:keys/admin.pub]
environments:
  production:
    nodes: [{ host: 127.0.0.1, port: $(port $C) }]
    sites:
      example.com: { domain: example.com, redirects: [www.example.com], ssl: none }
EOF
printf 'version: 1\nos: ubuntu-24.04\n' >"$P/trellis.lock"
export TRELLIS_AGE_KEY_FILE=$KEY # decrypt secrets with the Lima SSH key
(cd "$P" && rm -f secrets/production.yml.age && "$WORK/trellis" converge secrets init production >/dev/null &&
  printf 'smtp_password' | "$WORK/trellis" converge secrets set production mail_password >/dev/null)

converge() {
  (cd "$P" && "$WORK/trellis" converge apply production --yes \
    --ssh-option IdentityFile="$KEY" --ssh-option UserKnownHostsFile=/dev/null --ssh-option IdentitiesOnly=yes)
}

log "Provisioning both (logs: $WORK/ansible.log, $WORK/converge.log)"
(
  start=$SECONDS
  cd "$WORK/trellis" && ANSIBLE_FORCE_COLOR=0 "$VENV/ansible-playbook" server.yml -e env=production >"$WORK/ansible.log" 2>&1
  echo "stock Trellis: $((SECONDS - start))s"
) &
ansible=$!
(
  start=$SECONDS
  converge >"$WORK/converge.log" 2>&1
  echo "converge: $((SECONDS - start))s (first run)"
  start=$SECONDS
  converge >"$WORK/converge-2.log" 2>&1
  echo "converge: $((SECONDS - start))s (second run, as admin with sudo password): $(tail -1 "$WORK/converge-2.log")"
) &
conv=$!
wait $conv || { echo "converge failed; see $WORK/converge.log" >&2; exit 1; }
wait $ansible || { echo "Ansible failed; see $WORK/ansible.log" >&2; exit 1; }

log "Probing and comparing"
probe $A after & probe $C after & wait
"$WORK/fingerprint" compare -config "$CLI/pkg/converge/fingerprint/compare.yml" \
  "$WORK/$A.before.txt" "$WORK/$A.after.txt" "$WORK/$C.before.txt" "$WORK/$C.after.txt" >"$WORK/report.md"
sed -n '/total/p' "$WORK/report.md"
echo
echo "Report: $WORK/report.md (\"Accepted\" differences are intended; see compare.yml)"
