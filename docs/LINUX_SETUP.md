# wyndctl on Linux — Setup Notes

Notes for operators running `wyndctl` on Linux (tested on Ubuntu 22.04+). If `wyndctl scan` reports **"no Sentry devices found"** even though the device is plugged in, work through this checklist in order.

## 1. Build the binary on the Linux machine

`wyndctl` does USB serial discovery using OS-specific code paths. A binary built on macOS **will not** correctly enumerate serial ports on Linux, even though it runs without errors — it will silently report zero devices.

```bash
cd ~/code/wyndctl
make install
hash -r          # clear any cached path to an old binary
which wyndctl    # should be $GOPATH/bin/wyndctl
```

Do not copy a `wyndctl` binary from a Mac to a Linux machine. Always rebuild on the target OS.

## 2. Confirm the OS sees the device

Plug the Sentry in via USB, then:

```bash
lsusb | grep -i 2fe3            # expect: ID 2fe3:0100 NordicSemiconductor Wynd Sentry
ls -l /dev/ttyACM*              # expect: crw-rw---- root dialout /dev/ttyACM0
```

If `lsusb` shows nothing, it's a hardware problem (cable, port, hub) — not a `wyndctl` problem. Try a different cable; charge-only USB-C cables are a common culprit.

If `lsusb` shows the device but `/dev/ttyACM0` is missing, the `cdc_acm` kernel module isn't loaded:

```bash
sudo modprobe cdc_acm
```

## 3. Add yourself to the `dialout` group

`/dev/ttyACM0` is owned by `root:dialout`. Without group membership, `wyndctl` can find the device but cannot open it.

```bash
groups                                  # check if 'dialout' is listed
sudo usermod -aG dialout $USER          # add yourself if not
```

**You must fully log out and log back in** for the group change to take effect. Opening a new terminal tab is not enough — group membership is set at login.

After re-login, `groups` should include `dialout`.

## 4. (Optional) Tame ModemManager

On some Ubuntu installs, `ModemManager` briefly opens `/dev/ttyACM*` when a device is plugged in, which can make `wyndctl scan` flaky for the first few seconds after plug-in. If you see intermittent failures right after connecting a device:

```bash
sudo systemctl mask ModemManager
```

Sentry devices are not modems; nothing on this workstation needs ModemManager.

## 5. (Optional) Permanent udev rule

Instead of relying on `dialout` group membership, you can scope access to Sentry devices specifically with a udev rule. One-time setup:

```bash
sudo tee /etc/udev/rules.d/99-wynd-sentry.rules >/dev/null <<'EOF'
SUBSYSTEM=="tty", ATTRS{idVendor}=="2fe3", ATTRS{idProduct}=="0100", MODE="0660", GROUP="dialout"
EOF
sudo udevadm control --reload-rules
sudo udevadm trigger
```

This survives reboots and replugs, and limits permissions to Sentry hardware.

## Quick troubleshooting flow

```
wyndctl scan reports "no Sentry devices found"
│
├─ Did `lsusb | grep 2fe3` find the device?
│   ├─ No  → cable / port / hub problem. Stop here.
│   └─ Yes → continue
│
├─ Does `/dev/ttyACM0` exist?
│   ├─ No  → `sudo modprobe cdc_acm`
│   └─ Yes → continue
│
├─ Was `wyndctl` built on this Linux machine?
│   ├─ No  → `make install` on this host
│   └─ Yes → continue
│
└─ Is your user in `dialout`?
    ├─ No  → `sudo usermod -aG dialout $USER`, then full logout/login
    └─ Yes → run `sudo wyndctl scan`. If that works, repeat the logout/login.
             If even sudo fails, file an issue with the output of:
               wyndctl scan 2>&1
               ls -la /sys/class/tty/ttyACM0/device/../
               cat /sys/class/tty/ttyACM0/device/../idVendor
```

## Why "no devices found" is a misleading error

The same symptom — zero devices reported — is produced by at least three independent root causes:

1. Wrong-OS binary (discovery code can't read Linux sysfs)
2. Device not enumerated by the kernel (no `/dev/ttyACM*`)
3. Discovery succeeded but the open later failed (permissions) — depending on the code path, this can also surface as "no devices"

When in doubt, work through steps 1–3 above in order rather than guessing.
