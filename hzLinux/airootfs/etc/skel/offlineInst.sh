#!/usr/bin/env bash


cp -ax / /mnt
cp -vaT /run/archiso/bootmnt/arch/boot/$(uname -m)/vmlinuz /mnt/boot/vmlinuz-linux
genfstab -U /mnt >> /mnt/etc/fstab
# Whacky logic from here: https://askubuntu.com/questions/551195/scripting-chroot-how-to#551361
cat << EOF | arch-chroot /mnt /bin/bash
sed -i 's/Storage=volatile/#Storage=auto/' /etc/systemd/journald.conf
rm /etc/udev/rules.d/81-dhcpcd.rules
systemctl disable pacman-init.service choose-mirror.service
rm -r /etc/systemd/system/{choose-mirror.service,pacman-init.service,etc-pacman.d-gnupg.mount,getty@tty1.service.d}
rm /etc/systemd/scripts/choose-mirror
rm /etc/systemd/system/getty@tty1.service.d/autologin.conf
rm /root/{.automated_script.sh,.zlogin}
rm /etc/mkinitcpio-archiso.conf
rm -r /etc/initcpio
pacman-key --init
pacman-key --populate archlinux
exit
EOF
echo "Done, check your fstab!"
