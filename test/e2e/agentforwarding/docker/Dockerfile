# Two roles from one image: "bastion" is where the user opens a shell, "target" is the next host,
# reachable only with the forwarded key. Alpine 3.20 ships OpenSSH 9.7, which sends session-bind to
# the agent and authenticates with publickey-hostbound - the paths ADR-022 relies on.
FROM alpine:3.20

# adduser -D locks the account, and sshd without PAM refuses a locked account even for a key.
# Replacing "!" with "*" keeps password login impossible while leaving key login open.
# openssh-sftp-server is a separate package on Alpine, and scp has spoken SFTP since OpenSSH 9.0.
RUN apk add --no-cache openssh-server openssh-sftp-server openssh-client git rsync bash \
 && adduser -D -s /bin/bash ops \
 && adduser -D -s /usr/bin/git-shell git \
 && echo 'ops:bastion-password' | chpasswd \
 && sed -i 's/^git:!/git:*/' /etc/shadow

COPY entrypoint.sh /entrypoint.sh
RUN chmod 0755 /entrypoint.sh

EXPOSE 22
ENTRYPOINT ["/entrypoint.sh"]
