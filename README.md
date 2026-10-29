# appclean: macOS Uninstalled Application Support Cleaner

A fast, concurrent Go CLI that detects and removes leftover folders recursively for applications that are no longer installed on macOS—focusing on `Application Support` and associated library directories.

## Features

- **Concurrent Architecture**:
  - Worker pools concurrently index installed `.app` bundles across `/Applications`, `/System/Applications`, `~/Applications`, and Spotlight (`mdfind`).
  - Concurrently parses `Info.plist` (both XML and binary plists via native `plutil`).
  - Concurrently calculates folder sizes, counts files, and collects preview samples.
  - Concurrently deletes or trashes confirmed folders with real-time progress updates.
- **Accurate & Safe Matching Engine**:
  - Matches folders against GUI App Names, Bundle Identifiers (`CFBundleIdentifier`), Display Names, and Executables.
  - **CLI Tool & Developer Environment Detection**: Automatically discovers all installed and user-created command-line binaries in `$PATH`, `~/bin`, `~/.local/bin`, `~/go/bin`, `~/.cargo/bin`, `~/.bun/bin`, `/opt/homebrew/bin`, `/usr/local/bin`, etc., preventing CLI tools (like `2fa`, `go`, `git`, `gh`, `docker`, `ollama`, or user scripts) from being flagged.
  - Multi-application vendor folder handling (e.g. `Google/Chrome`, `Adobe/...`, `JetBrains/...`).
  - Whitelist of protected macOS system services, core daemons, frameworks, and developer toolchains (`AddressBook`, `CloudDocs`, `MobileSync`, `com.apple.*`, `go`, `2fa`, `git`, `docker`, `node`, `cargo`, etc.).
- **Interactive Confirmation**:
  - Lists orphaned folders with their inferred app names, full paths, disk space, file counts, and sample files.
  - Interactive confirmation prompt with choices:
    - `[y]` Delete all orphaned folders
    - `[s]` Select specific folders (e.g. `1, 3` or `2-4`)
    - `[d]` Toggle detailed file breakdown
    - `[n]` Cancel safely (default)
- **Safety First**:
  - `--dry-run` / `-n`: Preview everything without deleting.
  - `--trash`: Move to `~/.Trash` instead of immediate permanent deletion.
  - User exclusion support (`--exclude`).

---

## Installation & Build

Requires Go 1.22+:

```bash
# Clone or navigate to the fix_macos directory
cd fix_macos

# Build the binary
go build -o appclean ./cmd/appclean

# (Optional) Move to your PATH
sudo cp appclean /usr/local/bin/
```

---

## Usage

### 1. Dry Run Preview (Recommended first step)
Preview all orphaned application folders and reclaimable space without modifying any files:
```bash
./appclean -dry-run
```

### 2. Standard Interactive Cleanup
Scan `~/Library/Application Support` and `/Library/Application Support`, review uninstalled apps, and confirm deletion:
```bash
./appclean
```

### 3. Move to Trash Instead of Permanent Delete
If you prefer moving orphaned folders to macOS Trash:
```bash
./appclean -trash
```

### 4. Include Caches or Preferences
Extend scanning to `~/Library/Caches` and `/Library/Caches`:
```bash
./appclean -caches
```

Or include `~/Library/Preferences`:
```bash
./appclean -prefs
```

### 5. Filter by Minimum Size
Only list folders taking up more than 10 MB:
```bash
./appclean -min-size 10MB
```

### 6. Non-Interactive / Automation Mode
Skip confirmation prompt and delete automatically:
```bash
./appclean -yes
```

---

## CLI Options

| Flag | Shorthand | Description | Default |
|------|-----------|-------------|---------|
| `-dry-run` | `-n` | Preview orphaned folders without deleting | `false` |
| `-yes` | `-y` | Skip interactive prompt and delete automatically | `false` |
| `-trash` | | Move items to Trash (`~/.Trash`) instead of permanent removal | `false` |
| `-caches` | | Include user and system `Caches` directories | `false` |
| `-prefs` | | Include user `Preferences` directory | `false` |
| `-path` | `-p` | Custom comma-separated directories to scan | Standard Support Dirs |
| `-workers` | `-w` | Number of concurrent worker goroutines | `2 * CPU cores` |
| `-min-size` | | Minimum folder size filter (e.g. `500KB`, `10MB`, `1GB`) | `0` |
| `-exclude` | | Comma-separated list of names/patterns to keep | `""` |
| `-cli-dirs` | | Additional custom directories containing CLI binaries (comma-separated) | `""` |
| `-no-cli` | | Disable CLI tool matching | `false` |
| `-verbose` | `-v` | Show detailed logging during scanning | `false` |
| `-no-spotlight` | | Disable `mdfind` Spotlight query integration | `false` |

---

## macOS Permissions (Full Disk Access)

`~/Library/Application Support` contains protected user data governed by macOS Transparency, Consent, and Control (TCC). If running in a terminal emulator without Full Disk Access, you may see a warning.

To grant access:
1. Open **System Settings**.
2. Navigate to **Privacy & Security** > **Full Disk Access**.
3. Toggle the switch for your terminal application (e.g., Terminal, iTerm2, Ghostty).

---

## Running Tests

Run the full automated test suite:
```bash
go test -v ./...
```
