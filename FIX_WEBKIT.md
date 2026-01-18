# Fixing WebKit2GTK 4.0 vs 4.1 Issue

Wails v2 expects `webkit2gtk-4.0` but Ubuntu 24.04 only has `webkit2gtk-4.1` available.

## Solution: Create a pkg-config symlink

Create a symlink from `webkit2gtk-4.0.pc` to `webkit2gtk-4.1.pc`:

```bash
sudo ln -s /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.1.pc /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.0.pc
```

This allows pkg-config to find `webkit2gtk-4.0` when it's actually looking for `webkit2gtk-4.1`.

## Verify it works

```bash
pkg-config --exists webkit2gtk-4.0 && echo "OK" || echo "FAILED"
```

Should print "OK".

## Alternative: Use CGO environment variable

If you can't create the symlink, you can set PKG_CONFIG_PATH to point to a local pkg-config directory with the symlink.

