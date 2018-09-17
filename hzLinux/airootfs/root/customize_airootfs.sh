#!/bin/bash

set -e -u

USER=hzlinarch

sed -i 's/#\(en_US\.UTF-8\)/\1/' /etc/locale.gen
locale-gen

ln -sf /usr/share/zoneinfo/UTC /etc/localtime

usermod -s /usr/bin/zsh root
cp -aT /etc/skel/ /root/
chmod 700 /root

sed -i 's/#\(PermitRootLogin \).\+/\1yes/' /etc/ssh/sshd_config
sed -i "s/#Server/Server/g" /etc/pacman.d/mirrorlist
sed -i 's/#\(Storage=\)auto/\1volatile/' /etc/systemd/journald.conf

sed -i 's/#\(HandleSuspendKey=\)suspend/\1ignore/' /etc/systemd/logind.conf
sed -i 's/#\(HandleHibernateKey=\)hibernate/\1ignore/' /etc/systemd/logind.conf
sed -i 's/#\(HandleLidSwitch=\)suspend/\1ignore/' /etc/systemd/logind.conf

# Make a user
! id $USER && useradd -m -p "" -g users -G \
    "video,wheel,adm,audio,floppy,log,network,rfkill,scanner,storage,optical,power,wheel" \
    -s /usr/bin/zsh $USER
echo -e "$USER\n$USER" | passwd $USER
cp -aT /etc/skel/ /home/$USER
chmod -R 751 /home/$USER
chown -R $USER /home/$USER

systemctl enable pacman-init.service choose-mirror.service

# Added
systemctl enable reflector.service \
    NetworkManager.service \
    thermald.service \
    udisks2.service \
    lightdm.service

systemctl set-default graphical.target
