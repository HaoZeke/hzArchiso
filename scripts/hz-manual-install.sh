#!/usr/bin/env bash
# Manual encrypted-btrfs install for rgam5terra. NO archinstall.
# Prompts for LUKS / root / user passwords (you type them).
set -euo pipefail

DISK=/dev/nvme0n1
ESP=${DISK}p1
CRYPT=${DISK}p2
MAP=cryptroot
HOST=rgam5terra
PUSER=rgoswami
TZ=America/Chicago
MNT=/mnt
BO="noatime,compress=zstd,ssd,space_cache=v2"

echo "==================== PARTITION PLAN ===================="
echo "TARGET: $DISK  (THIS WIPES THE WHOLE DISK)"
lsblk "$DISK"
echo " p1 = 1 GiB  EFI (fat32)            -> /boot"
echo " p2 = rest   LUKS2 -> btrfs         -> / (subvols @ @home @snapshots @var_log @cache)"
echo " bootloader: systemd-boot ; hooks: encrypt+btrfs ; ucode: amd"
echo "======================================================="
read -r -p "Type exactly WIPE to proceed: " CONF
[ "$CONF" = "WIPE" ] || { echo "aborted"; exit 1; }

read -rs -p "LUKS encryption passphrase: " LUKSPW; echo
read -rs -p "  confirm LUKS passphrase : " LUKSPW2; echo
[ "$LUKSPW" = "$LUKSPW2" ] && [ -n "$LUKSPW" ] || { echo "LUKS mismatch/empty"; exit 1; }
read -rs -p "root password: " ROOTPW; echo
read -rs -p "$PUSER password: " USERPW; echo
[ -n "$ROOTPW" ] && [ -n "$USERPW" ] || { echo "empty pw"; exit 1; }

echo "=== partitioning ==="
umount -R "$MNT" 2>/dev/null || true
cryptsetup close "$MAP" 2>/dev/null || true
sgdisk --zap-all "$DISK"
sgdisk -n1:0:+1GiB -t1:ef00 -c1:EFI "$DISK"
sgdisk -n2:0:0     -t2:8309 -c2:cryptroot "$DISK"
partprobe "$DISK"; sleep 2

echo "=== LUKS ==="
printf '%s' "$LUKSPW" | cryptsetup luksFormat --type luks2 --batch-mode "$CRYPT" -
printf '%s' "$LUKSPW" | cryptsetup open "$CRYPT" "$MAP" -

echo "=== filesystems + subvols ==="
mkfs.fat -F32 "$ESP"
mkfs.btrfs -f -L hzroot "/dev/mapper/$MAP"
mount "/dev/mapper/$MAP" "$MNT"
for s in @ @home @snapshots @var_log @cache; do btrfs subvolume create "$MNT/$s"; done
umount "$MNT"
mount -o "$BO,subvol=@" "/dev/mapper/$MAP" "$MNT"
mkdir -p "$MNT"/{home,.snapshots,var/log,var/cache/pacman/pkg,boot}
mount -o "$BO,subvol=@home"      "/dev/mapper/$MAP" "$MNT/home"
mount -o "$BO,subvol=@snapshots" "/dev/mapper/$MAP" "$MNT/.snapshots"
mount -o "$BO,subvol=@var_log"   "/dev/mapper/$MAP" "$MNT/var/log"
mount -o "$BO,subvol=@cache"     "/dev/mapper/$MAP" "$MNT/var/cache/pacman/pkg"
mount "$ESP" "$MNT/boot"

echo "=== pacstrap (bootable encrypted base) ==="
pacstrap -K "$MNT" base base-devel linux linux-lts linux-firmware amd-ucode \
  btrfs-progs cryptsetup sudo vim git openssh networkmanager terminus-font

echo "=== fstab ==="
genfstab -U "$MNT" >> "$MNT/etc/fstab"

CRYPT_UUID=$(blkid -s UUID -o value "$CRYPT")
echo "=== chroot config (crypt UUID=$CRYPT_UUID) ==="
arch-chroot "$MNT" /bin/bash -s -- "$HOST" "$PUSER" "$TZ" "$CRYPT_UUID" "$ROOTPW" "$USERPW" <<'CHROOT'
set -euo pipefail
HOST="$1"; PUSER="$2"; TZ="$3"; CRYPT_UUID="$4"; ROOTPW="$5"; USERPW="$6"
ln -sf "/usr/share/zoneinfo/$TZ" /etc/localtime; hwclock --systohc || true
sed -i 's/^#en_US.UTF-8/en_US.UTF-8/' /etc/locale.gen; locale-gen
echo "LANG=en_US.UTF-8" > /etc/locale.conf
echo "$HOST" > /etc/hostname
printf '127.0.0.1 localhost\n::1 localhost\n127.0.1.1 %s\n' "$HOST" > /etc/hosts
# encrypt + btrfs hooks
sed -i 's/^HOOKS=.*/HOOKS=(base udev autodetect microcode modconf kms keyboard keymap consolefont block encrypt filesystems fsck)/' /etc/mkinitcpio.conf
mkinitcpio -P
echo "root:$ROOTPW" | chpasswd
useradd -m -G wheel -s /bin/bash "$PUSER"
echo "$PUSER:$USERPW" | chpasswd
sed -i 's/^# %wheel ALL=(ALL:ALL) ALL/%wheel ALL=(ALL:ALL) ALL/' /etc/sudoers
systemctl enable NetworkManager sshd
bootctl install
cat > /boot/loader/loader.conf <<L
default arch.conf
timeout 3
console-mode max
L
CMD="cryptdevice=UUID=${CRYPT_UUID}:cryptroot root=/dev/mapper/cryptroot rootflags=subvol=@ rw"
cat > /boot/loader/entries/arch.conf <<E
title   hzLinux (rgam5terra)
linux   /vmlinuz-linux
initrd  /amd-ucode.img
initrd  /initramfs-linux.img
options $CMD
E
cat > /boot/loader/entries/arch-lts.conf <<E
title   hzLinux (rgam5terra, LTS)
linux   /vmlinuz-linux-lts
initrd  /amd-ucode.img
initrd  /initramfs-linux-lts.img
options $CMD
E
CHROOT

echo "=== DONE: encrypted-btrfs rgam5terra base installed. ==="
echo "Unmount with: umount -R $MNT && cryptsetup close $MAP ; then reboot (remove USB)."
