<div align="center">
  
![GusSync Logo](logo.png)
  
</div>

# 
<img src="bullet.png" width="16" height="16"> GusSync

**"Digs deep. Fetches everything. Never lets go."**

GusSync is a CLI backup tool inspired by my Border Terrier, Gus.

If you have ever tried to back up a large Android phone on Linux via MTP/FUSE, you know the pain. You mount the drive. You drag the folder. The file manager freezes. It tries to "prepare" the copy for 20 minutes. Then it times out. You try again. It fails halfway through. You scream at your monitor.

I built GusSync because I was tired of being polite to a fragile filesystem. I needed a tool that acts like a terrier with a flirt pole: **ravenous, high-energy, and completely unwilling to give up.**

### Why is this different?

Standard file managers (Nemo, Nautilus) try to be elegant. They want to list every single file before they copy one byte. On a phone with 100,000 photos, this causes the connection to hang.

**GusSync is not elegant. It is brute force.**

* 
  <img src="bullet.png" width="16" height="16"> **It Attacks Immediately:** It starts copying the *second* it finds a file. It doesn't wait to map the whole tree.

* 
  <img src="bullet.png" width="16" height="16"> **It Plays Tug-of-War:** If a file transfer stalls or the connection drops, GusSync doesn't crash. It detects the slack line, drops the bad thread, and lunges again.

* 
  <img src="bullet.png" width="16" height="16"> **It Remembers the Scent:** It tracks every single file in a Markdown checklist (`gus_state.md`). If the process dies (or you rage-quit), you just run it again. It sees what it already fetched and immediately resumes digging for new files.

---

## 
<img src="bullet.png" width="16" height="16"> Key Features

* 
  <img src="bullet.png" width="16" height="16"> **The "Chase" Mode (Streaming):** Discovers and copies files simultaneously. No waiting for "calculating time remaining..."

* 
  <img src="bullet.png" width="16" height="16"> **Dual Mode Hunting:**

    * **`mount` mode:** For when you want to fight with the standard filesystem paths (MTP/GVFS/gphoto2).

    * **`adb` mode:** The "Nuclear Option." Bypasses the filesystem entirely and uses the Android Debug Bridge to pull files directly.

* 
  <img src="bullet.png" width="16" height="16"> **Total Recall:** Uses a human-readable Markdown file to track progress. It knows exactly what it has already buried in your hard drive.

* 
  <img src="bullet.png" width="16" height="16"> **The Guard Dog (Health Monitoring):** checks the connection every 30 seconds. If the phone disconnects, it logs the error and exits gracefully so you can reconnect and resume instantly.

* 
  <img src="bullet.png" width="16" height="16"> **Scent Verification:** Counts files found vs. files copied. It knows if it missed something.

* 
  <img src="bullet.png" width="16" height="16"> **Stall Detection:** If a file transfer hangs for 30 seconds, GusSync cuts the line and retries.

## 
<img src="bullet.png" width="16" height="16"> Installation

**Prerequisites**

* Go 1.18 or later

* Linux (Ubuntu/Debian recommended)

* *A stubborn attitude*

**Build**

```bash
git clone <repository-url>
cd GusSync
go build -o gussync .
```

## Usage

### Basic Usage

```bash
./gussync -source <source_path> -dest <dest_path> -mode <mount|adb> [-workers <num>]
```

### Examples

**Mount Mode (MTP/gphoto2):**
```bash
./gussync -source /run/user/1000/gvfs/gphoto2:host=Your_Device_Name \
          -dest /mnt/backup/phone \
          -mode mount \
          -workers 1
```

**ADB Mode:**
```bash
./gussync -source /sdcard \
          -dest /mnt/backup/phone \
          -mode adb \
          -workers 2
```

### Flags

- `-source`: Source directory path
  - For `mount` mode: Local filesystem path (e.g., `/run/user/1000/gvfs/mtp:host=...`)
  - For `adb` mode: Android path (e.g., `/sdcard`)
- `-dest`: Destination directory (local filesystem)
- `-mode`: Backup mode - `mount` or `adb` (default: `mount`)
- `-workers`: Number of worker threads (default: 1)

### Test Script

Use the provided test script for easier execution:

```bash
./test_mtp.sh mount 1
# or
./test_mtp.sh adb 2
```

## Recent Changes

### Connection Health Monitoring (Latest)
* 
  <img src="bullet.png" width="16" height="16"> **Periodic connection checks**: Every 30 seconds, verifies source path is still accessible
* 
  <img src="bullet.png" width="16" height="16"> **Connection drop detection**: Automatically detects when MTP/gphoto2 connection drops
* 
  <img src="bullet.png" width="16" height="16"> **Graceful exit**: Exits cleanly with error message instead of silently failing
* 
  <img src="bullet.png" width="16" height="16"> **Progress preservation**: State is flushed before exit, allowing resume

### Discovery Verification (Latest)
* 
  <img src="bullet.png" width="16" height="16"> **File discovery counting**: Tracks how many files were discovered during scan
* 
  <img src="bullet.png" width="16" height="16"> **Actual file count comparison**: After backup, counts actual files in source directory
* 
  <img src="bullet.png" width="16" height="16"> **Missing file warnings**: Warns if discovered count < actual count with percentage missing
* 
  <img src="bullet.png" width="16" height="16"> **Helps detect incomplete scans**: Alerts when directories timeout or fail during scanning

### Verification Improvements
* 
  <img src="bullet.png" width="16" height="16"> **Source path filtering**: Verification now only checks files from current source path
* 
  <img src="bullet.png" width="16" height="16"> **Prevents cross-mount verification**: Filters out files from previous runs with different mount points
* 
  <img src="bullet.png" width="16" height="16"> **Progress display**: Shows verification progress with file count and percentage

## Troubleshooting (Linux)

### ADB Mode Setup

#### 1. Install ADB

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install adb
```

**Verify installation:**
```bash
adb version
```

#### 2. Enable Developer Mode on Xiaomi Mi 11 Ultra

1. **Unlock Developer Options:**
   - Go to `Settings` → `About phone`
   - Tap `MIUI version` 7 times rapidly
   - You'll see "You are now a developer!"

2. **Enable USB Debugging:**
   - Go to `Settings` → `Additional settings` → `Developer options`
   - Enable `USB debugging`
   - Enable `Install via USB` (if present)
   - For newer MIUI versions, also enable `USB debugging (Security settings)`

3. **Authorize Computer:**
   - Connect phone via USB
   - When prompted on phone: "Allow USB debugging?", check "Always allow from this computer" and tap "OK"

4. **Set USB Configuration:**
   - While connected, pull down notification shade
   - Tap "USB" or "Charging this device via USB"
   - Select "File Transfer" or "MTP" mode

#### 3. Verify ADB Connection

```bash
# List connected devices
adb devices

# Expected output:
# List of devices attached
# abc123def    device
```

If device shows `unauthorized`:
- Check phone for authorization prompt
- Revoke USB debugging authorizations in Developer options and reconnect

If device doesn't appear:
- Try different USB cable (data-capable cable required)
- Try different USB port (USB 2.0 ports sometimes work better)
- Restart ADB server: `adb kill-server && adb start-server`
- Check `lsusb` for phone presence:
  ```bash
  lsusb | grep -i xiaomi
  ```

### Mount Mode Setup

#### Finding Mount Points

**MTP mounts (Nautilus/File Manager):**
```bash
# MTP mounts are typically at:
ls -la /run/user/$UID/gvfs/

# Look for:
# mtp:host=Your_Device_Name
```

**gphoto2 mounts (Picture mode):**
```bash
# gphoto2 mounts are typically at:
ls -la /run/user/$UID/gvfs/

# Look for:
# gphoto2:host=Your_Device_Name
```

**Check if mounted:**
```bash
# List all GVFS mounts
gio mount -l

# Or check mount point directly
ls /run/user/1000/gvfs/gphoto2:host=Your_Device_Name/
```

#### Mount Troubleshooting

**If mount point doesn't exist:**

1. **Install GVFS and MTP support:**
   ```bash
   sudo apt install gvfs-backends gvfs-fuse
   ```

2. **Unmount and remount:**
   ```bash
   # Unmount
   gio mount -u mtp://[device]
   
   # Or use Nautilus/file manager to safely eject
   ```

3. **Restart user services:**
   ```bash
   # Logout and login again, or:
   systemctl --user restart gvfs-daemon
   ```

4. **Check udev rules (if needed):**
   ```bash
   # Create udev rule for your device
   sudo nano /etc/udev/rules.d/51-android.rules
   # Add:
   # SUBSYSTEM=="usb", ATTR{idVendor}=="2717", MODE="0664", GROUP="plugdev"
   # (Replace 2717 with your device's vendor ID from lsusb)
   sudo udevadm control --reload-rules
   ```

### Connection Issues

#### Problem: "Backup complete" but missing many files

**Symptoms:**
* 
  <img src="bullet.png" width="16" height="16"> Tool reports backup complete
* 
  <img src="bullet.png" width="16" height="16"> Actual file count is much higher than discovered count
* 
  <img src="bullet.png" width="16" height="16"> Verification shows files missing from source

**Solutions:**

1. **Check error log:**
   ```bash
   tail -f /path/to/dest/mount/gus_errors.log
   ```
   Look for "directory read timeout" or "CRITICAL: Connection dropped" messages

2. **Increase workers gradually:**
   ```bash
   # Start with 1 worker (most stable)
   ./gussync -source ... -dest ... -mode mount -workers 1
   
   # If stable, try 2-4 workers
   ./gussync -source ... -dest ... -mode mount -workers 2
   ```

3. **Check connection health:**
   - Monitor the health check messages (runs every 30s)
   - If connection drops, reconnect and resume

4. **Use ADB mode instead:**
   - ADB is often more stable than MTP/gphoto2 mounts
   - Switch to `-mode adb` for more reliable operation

#### Problem: Connection keeps dropping

**Symptoms:**
* 
  <img src="bullet.png" width="16" height="16"> "CRITICAL: Connection dropped" errors
* 
  <img src="bullet.png" width="16" height="16"> Backup stops mid-way
* 
  <img src="bullet.png" width="16" height="16"> Files timeout or fail to copy

**Solutions:**

1. **Check USB connection:**
   * 
  <img src="bullet.png" width="16" height="16"> Use a high-quality USB cable (data-capable, not charge-only)
   * 
  <img src="bullet.png" width="16" height="16"> Try different USB ports (prefer USB 2.0 ports)
   * 
  <img src="bullet.png" width="16" height="16"> Avoid USB hubs - connect directly to computer

2. **Check power management:**
   * 
  <img src="bullet.png" width="16" height="16"> Disable USB selective suspend:
     ```bash
     # Check current setting
     cat /sys/module/usbcore/parameters/usbfs_memory_mb
     
     # Disable selective suspend (temporary)
     sudo sh -c 'echo 0 > /sys/module/usbcore/parameters/usbfs_memory_mb'
     ```

3. **Check phone power settings:**
   * 
  <img src="bullet.png" width="16" height="16"> Disable "Battery saver" mode during backup
   * 
  <img src="bullet.png" width="16" height="16"> Keep phone screen on (prevents Android from suspending USB)

4. **Reduce workers:**
   ```bash
   # Use 1 worker for maximum stability
   ./gussync ... -workers 1
   ```

5. **Monitor connection:**
   * 
  <img src="bullet.png" width="16" height="16"> Watch `gus_errors.log` for connection errors
   * 
  <img src="bullet.png" width="16" height="16"> Check if mount point still accessible:
     ```bash
     ls /run/user/1000/gvfs/gphoto2:host=Your_Device_Name/
     ```

#### Problem: Directories timeout during scanning

**Symptoms:**
* 
  <img src="bullet.png" width="16" height="16"> Error log shows "directory read timeout: /path/to/dir"
* 
  <img src="bullet.png" width="16" height="16"> Files in those directories are not discovered
* 
  <img src="bullet.png" width="16" height="16"> Discovered count < actual file count

**Solutions:**

1. **Check MTP/gphoto2 mount stability:**
   ```bash
   # Try listing a problematic directory manually
   ls -la "/run/user/1000/gvfs/gphoto2:host=.../DCIM/"
   # If this hangs or fails, the mount is unstable
   ```

2. **Remount the device:**
   * 
  <img src="bullet.png" width="16" height="16"> Eject device from file manager
   * 
  <img src="bullet.png" width="16" height="16"> Reconnect and remount
   * 
  <img src="bullet.png" width="16" height="16"> Try backup again

3. **Use ADB mode:**
   * 
  <img src="bullet.png" width="16" height="16"> ADB is generally more reliable for large directory trees
   * 
  <img src="bullet.png" width="16" height="16"> Switch to `-mode adb` for better stability

4. **Resume after timeout:**
   * 
  <img src="bullet.png" width="16" height="16"> The tool saves progress in `gus_state.md`
   * 
  <img src="bullet.png" width="16" height="16"> Simply run again - it will skip already-copied files
   * 
  <img src="bullet.png" width="16" height="16"> Multiple runs may be needed to catch all files

#### Problem: ADB device not found

**Symptoms:**
* 
  <img src="bullet.png" width="16" height="16"> `adb devices` shows no devices
* 
  <img src="bullet.png" width="16" height="16"> "adb command not found" error

**Solutions:**

1. **Check ADB installation:**
   ```bash
   which adb
   adb version
   ```

2. **Restart ADB server:**
   ```bash
   adb kill-server
   adb start-server
   adb devices
   ```

3. **Check USB authorization:**
   * 
  <img src="bullet.png" width="16" height="16"> Look for authorization prompt on phone
   * 
  <img src="bullet.png" width="16" height="16"> Check "Always allow from this computer"

4. **Check USB mode:**
   * 
  <img src="bullet.png" width="16" height="16"> Phone must be in "File Transfer" or "MTP" mode
   * 
  <img src="bullet.png" width="16" height="16"> Not "Charging only" mode

5. **Check udev rules:**
   ```bash
   # Check if device is recognized
   lsusb | grep -i xiaomi
   
   # If vendor ID shows, create udev rule (see mount troubleshooting)
   ```

### General Tips

* 
  <img src="bullet.png" width="16" height="16"> **Start with 1 worker**: MTP/gphoto2 protocols are sensitive to concurrent connections. Start with `-workers 1` and increase only if stable.

* 
  <img src="bullet.png" width="16" height="16"> **Monitor error log**: Keep `gus_errors.log` open during backup:
   ```bash
   tail -f /path/to/dest/mount/gus_errors.log
   ```

* 
  <img src="bullet.png" width="16" height="16"> **Check state file**: Progress is saved in `gus_state.md`. You can inspect it to see what's been backed up.

* 
  <img src="bullet.png" width="16" height="16"> **Resume is automatic**: If backup is interrupted, simply run the same command again. It will skip already-copied files.

* 
  <img src="bullet.png" width="16" height="16"> **Use ADB for reliability**: If MTP/gphoto2 is unstable, ADB mode is often more reliable for large backups.

* 
  <img src="bullet.png" width="16" height="16"> **Verify after backup**: The tool automatically verifies all files after backup. Check the verification results for any issues.

## Files Created

* 
  <img src="bullet.png" width="16" height="16"> `gus_state.md`: Markdown file tracking completed files and their hashes
* 
  <img src="bullet.png" width="16" height="16"> `gus_errors.log`: Error log with timestamps for all errors
* 
  <img src="bullet.png" width="16" height="16"> Both files are in the destination directory (under `mount/` or `adb/` subdirectory)

## Architecture

For detailed architecture documentation, see [ARCHITECTURE.md](ARCHITECTURE.md).

## License

[Your License Here]

## Contributing

[Contributing Guidelines Here]

