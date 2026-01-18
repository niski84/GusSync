# GusSync

A robust, lightweight CLI tool for backing up files from phone filesystems mounted via MTP/FUSE on Linux. Designed to handle large filesystems efficiently without enumerating the entire tree before copying.

## Features

- **Streaming Architecture**: Discovers and copies files simultaneously (no sequential enumeration)
- **Dual Mode Support**: 
  - `mount` mode: Uses filesystem paths (MTP/FUSE/GVFS/gphoto2)
  - `adb` mode: Uses Android Debug Bridge for file discovery and copying
- **Resumable Backups**: Tracks progress in human-readable Markdown (`gus_state.md`)
- **Connection Health Monitoring**: Automatically detects connection drops and exits gracefully
- **Discovery Verification**: Counts and verifies discovered files vs actual files in source
- **Intelligent Prioritization**: Processes common Android paths first (DCIM, Camera, Documents, etc.)
- **Stall Detection**: Automatically handles stalled file transfers (30s timeout)
- **Hash Verification**: SHA256 hash verification for all files with automatic re-copy on mismatch
- **Error Logging**: All errors logged to `gus_errors.log` with timestamps
- **Real-time Progress**: Live statistics with per-worker status, transfer speeds, and MB delta

## Installation

### Prerequisites

- Go 1.18 or later
- Linux (tested on Ubuntu/Debian)

### Build

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
./gussync -source /run/user/1000/gvfs/gphoto2:host=Xiaomi_Mi_11_Ultra_8997acf \
          -dest /media/nick/New\ Volume/2026/phone\ backup \
          -mode mount \
          -workers 1
```

**ADB Mode:**
```bash
./gussync -source /sdcard \
          -dest /media/nick/New\ Volume/2026/phone\ backup \
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
- **Periodic connection checks**: Every 30 seconds, verifies source path is still accessible
- **Connection drop detection**: Automatically detects when MTP/gphoto2 connection drops
- **Graceful exit**: Exits cleanly with error message instead of silently failing
- **Progress preservation**: State is flushed before exit, allowing resume

### Discovery Verification (Latest)
- **File discovery counting**: Tracks how many files were discovered during scan
- **Actual file count comparison**: After backup, counts actual files in source directory
- **Missing file warnings**: Warns if discovered count < actual count with percentage missing
- **Helps detect incomplete scans**: Alerts when directories timeout or fail during scanning

### Verification Improvements
- **Source path filtering**: Verification now only checks files from current source path
- **Prevents cross-mount verification**: Filters out files from previous runs with different mount points
- **Progress display**: Shows verification progress with file count and percentage

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
# 8997acf    device
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
# mtp:host=Xiaomi_Mi_11_Ultra_8997acf
```

**gphoto2 mounts (Picture mode):**
```bash
# gphoto2 mounts are typically at:
ls -la /run/user/$UID/gvfs/

# Look for:
# gphoto2:host=Xiaomi_Mi_11_Ultra_8997acf
```

**Check if mounted:**
```bash
# List all GVFS mounts
gio mount -l

# Or check mount point directly
ls /run/user/1000/gvfs/gphoto2:host=Xiaomi_Mi_11_Ultra_8997acf/
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
- Tool reports backup complete
- Actual file count is much higher than discovered count
- Verification shows files missing from source

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
- "CRITICAL: Connection dropped" errors
- Backup stops mid-way
- Files timeout or fail to copy

**Solutions:**

1. **Check USB connection:**
   - Use a high-quality USB cable (data-capable, not charge-only)
   - Try different USB ports (prefer USB 2.0 ports)
   - Avoid USB hubs - connect directly to computer

2. **Check power management:**
   - Disable USB selective suspend:
     ```bash
     # Check current setting
     cat /sys/module/usbcore/parameters/usbfs_memory_mb
     
     # Disable selective suspend (temporary)
     sudo sh -c 'echo 0 > /sys/module/usbcore/parameters/usbfs_memory_mb'
     ```

3. **Check phone power settings:**
   - Disable "Battery saver" mode during backup
   - Keep phone screen on (prevents Android from suspending USB)

4. **Reduce workers:**
   ```bash
   # Use 1 worker for maximum stability
   ./gussync ... -workers 1
   ```

5. **Monitor connection:**
   - Watch `gus_errors.log` for connection errors
   - Check if mount point still accessible:
     ```bash
     ls /run/user/1000/gvfs/gphoto2:host=Xiaomi_Mi_11_Ultra_8997acf/
     ```

#### Problem: Directories timeout during scanning

**Symptoms:**
- Error log shows "directory read timeout: /path/to/dir"
- Files in those directories are not discovered
- Discovered count < actual file count

**Solutions:**

1. **Check MTP/gphoto2 mount stability:**
   ```bash
   # Try listing a problematic directory manually
   ls -la "/run/user/1000/gvfs/gphoto2:host=.../DCIM/"
   # If this hangs or fails, the mount is unstable
   ```

2. **Remount the device:**
   - Eject device from file manager
   - Reconnect and remount
   - Try backup again

3. **Use ADB mode:**
   - ADB is generally more reliable for large directory trees
   - Switch to `-mode adb` for better stability

4. **Resume after timeout:**
   - The tool saves progress in `gus_state.md`
   - Simply run again - it will skip already-copied files
   - Multiple runs may be needed to catch all files

#### Problem: ADB device not found

**Symptoms:**
- `adb devices` shows no devices
- "adb command not found" error

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
   - Look for authorization prompt on phone
   - Check "Always allow from this computer"

4. **Check USB mode:**
   - Phone must be in "File Transfer" or "MTP" mode
   - Not "Charging only" mode

5. **Check udev rules:**
   ```bash
   # Check if device is recognized
   lsusb | grep -i xiaomi
   
   # If vendor ID shows, create udev rule (see mount troubleshooting)
   ```

### General Tips

1. **Start with 1 worker**: MTP/gphoto2 protocols are sensitive to concurrent connections. Start with `-workers 1` and increase only if stable.

2. **Monitor error log**: Keep `gus_errors.log` open during backup:
   ```bash
   tail -f /path/to/dest/mount/gus_errors.log
   ```

3. **Check state file**: Progress is saved in `gus_state.md`. You can inspect it to see what's been backed up.

4. **Resume is automatic**: If backup is interrupted, simply run the same command again. It will skip already-copied files.

5. **Use ADB for reliability**: If MTP/gphoto2 is unstable, ADB mode is often more reliable for large backups.

6. **Verify after backup**: The tool automatically verifies all files after backup. Check the verification results for any issues.

## Files Created

- `gus_state.md`: Markdown file tracking completed files and their hashes
- `gus_errors.log`: Error log with timestamps for all errors
- Both files are in the destination directory (under `mount/` or `adb/` subdirectory)

## Architecture

For detailed architecture documentation, see [ARCHITECTURE.md](ARCHITECTURE.md).

## License

[Your License Here]

## Contributing

[Contributing Guidelines Here]

