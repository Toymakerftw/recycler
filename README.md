# Recycler CLI

**Recycler CLI** is a safer alternative to the traditional `rm` command. Instead of permanently deleting files, it moves them to a designated recycle bin directory mounted on a disk, preserving them for future recovery if needed. Additionally, Recycler CLI can monitor specific files for changes and create automated backups when modifications occur, ensuring data integrity.

## Features

- **Recycle Instead of Delete**: Moves specified files to a centralized recycle bin instead of deleting them permanently.
- **Configurable Settings**: Modify paths, file monitoring, and backup settings through the `config.conf` file for flexible, tailored use.
- **Restore Deleted**: Restore Deleted files.

## Installation

To install Recycler CLI with a single command:
```bash
curl -sSL https://github.com/Toymakerftw/recycler/raw/refs/heads/go/scripts/install.sh | sudo bash
```

> **Configuration File**: `/etc/recycler-cli/config.conf`  
> **Log File**: `/var/log/recycler-cli/recycler-cli.log`

## Usage

Run Recycler CLI to move files to the recycle bin:
```bash
recycler-cli -f <file1>,<file2>,...
```

### Example:
```bash
recycler-cli -f /path/to/file1.txt,/path/to/file2.log
```

### Options:
- `-f`: Specify a comma-separated list of files to move to the recycle bin.
- `-h`: Show help information.
- `-r`: Restore files from recycle bin.
- `-d`: Date to restore files from (format: YYYY-MM-DD).
- `-s`: Specify a single file to restore from the recycle bin on a given date.

### Configuration
The `config.conf` file (located by default at `/etc/recycler-cli/config.conf`) allows for adjusting key settings:
```json
{
  "recycleBinDir": "/mnt/recycle-bin",
  "numWorkers": 4
}
```

- **recycleBinDir**: The directory where files are moved instead of being deleted.
- **numWorkers**: Number of worker threads for handling multiple files simultaneously.

## Upcoming Features (To-dos)

- **File Monitoring**: Use `fsnotify` to monitor specified files for changes such as creation, modification, or deletion.
- **Automated Backups**: Automatically back up modified files to a designated backup directory.
- **Health Check API**: Implement an API to check the health status of the Recycler CLI service.

## Uninstallation

To uninstall Recycler CLI, use:
```bash
curl -sSL https://github.com/Toymakerftw/recycler/raw/refs/heads/go/scripts/uninstall.sh | sudo bash
```

During uninstallation, you will be prompted to remove the log directory and recycle bin directory if desired.

---

**Note:** Ensure that the recycle bin directory (`/mnt/recycle-bin`) has write permissions for the user running Recycler CLI.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.

---

## Contributing

Feel free to open issues or submit pull requests if you’d like to improve this project!

---

## Author

Developed by Anandhraman. 🚀
