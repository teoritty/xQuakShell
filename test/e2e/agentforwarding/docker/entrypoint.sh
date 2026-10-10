#!/bin/sh
# Installs the seed the test generated into the places sshd reads, with the ownership and modes
# StrictModes demands, then runs sshd in the foreground. The seed is bind-mounted read-only and
# arrives with whatever modes the host filesystem gives it, which is why it is copied, not used.
set -eu

ROLE="${ROLE:?ROLE must be bastion or target}"
SEED=/seed

install_file() { # src dest owner mode
	install -D -o "$3" -g "$3" -m "$4" "$1" "$2"
}

if [ "$ROLE" = target ]; then
	install_file "$SEED/target_host_ed25519_key" /etc/ssh/ssh_host_ed25519_key root 0600
	install_file "$SEED/target_host_ed25519_key.pub" /etc/ssh/ssh_host_ed25519_key.pub root 0644
	for user in ops git; do
		install -d -o "$user" -g "$user" -m 0700 "/home/$user/.ssh"
		install_file "$SEED/forwarded_key.pub" "/home/$user/.ssh/authorized_keys" "$user" 0600
	done
	# A private repository with one commit, so a clone has something to fetch.
	su -s /bin/sh git -c '
		git init -q --bare -b main /home/git/repo.git &&
		tmp=$(mktemp -d) && cd "$tmp" && git init -q -b main &&
		git -c user.name=e2e -c user.email=e2e@example.invalid commit -q --allow-empty -m seed &&
		git push -q /home/git/repo.git main && rm -rf "$tmp"'
	cat >/etc/ssh/sshd_config.d/e2e.conf <<'EOF'
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
HostKey /etc/ssh/ssh_host_ed25519_key
EOF
else
	install_file "$SEED/bastion_host_ed25519_key" /etc/ssh/ssh_host_ed25519_key root 0600
	install_file "$SEED/bastion_host_ed25519_key.pub" /etc/ssh/ssh_host_ed25519_key.pub root 0644
	install -d -o ops -g ops -m 0700 /home/ops/.ssh
	install_file "$SEED/bastion_known_hosts" /home/ops/.ssh/known_hosts ops 0644
	cat >/home/ops/.ssh/config <<'EOF'
Host target
	User ops
	StrictHostKeyChecking yes
	BatchMode yes
EOF
	chown ops:ops /home/ops/.ssh/config && chmod 0600 /home/ops/.ssh/config
	cat >/etc/ssh/sshd_config.d/e2e.conf <<'EOF'
PasswordAuthentication yes
AllowAgentForwarding yes
HostKey /etc/ssh/ssh_host_ed25519_key
EOF
fi

exec /usr/sbin/sshd -D -e
