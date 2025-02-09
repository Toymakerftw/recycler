# <img src="https://raw.githubusercontent.com/Toymakerftw/recycler/refs/heads/wip/banner.png" > Cbin
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT) [![Go Report Card](https://goreportcard.com/badge/github.com/Toymakerftw/recycler)](https://goreportcard.com/report/github.com/Toymakerftw/recycler) [![Go Version](https://img.shields.io/badge/Go-1.18-blue.svg)](https://golang.org/doc/go1.18) [![Ubuntu Version](https://img.shields.io/badge/Ubuntu-24.04-orange.svg)](https://ubuntu.com/) ![Visitor Count](https://visitor-badge.laobi.icu/badge?page_id=Toymakerftw.recycler)

**Cbin CLI** offers a safer alternative to the traditional `rm` command. Instead of permanently deleting files, it moves them to a centralized recycle bin directory, allowing for easy recovery when needed.

## Features

- **Recycle Instead of Delete**: Moves files to a designated recycle bin instead of permanently deleting them.
- **Configurable Settings**: Customize paths and other options through the `config.conf` file.
- **File Restoration**: Restore files from the recycle bin effortlessly.
- **Centralized Node Management**: Add or remove agent nodes seamlessly via the master dashboard.

## Installation

Install Cbin CLI:

```bash
curl -sSL -o install.sh https://github.com/Toymakerftw/recycler/raw/refs/heads/wip/agent/bin/install.sh
chmod +x install.sh
sudo ./install.sh
```

### Command Options

- `-rf`, `--force-remove`: Forcefully remove files or directories (use cautiously).
- `-f`, `--files`: Specify a comma-separated list of files to recycle (e.g., `file1.txt,file2.log`).
- `-restore`, `--restore`: Restore files from the recycle bin.
- `-d`, `--date`: Specify a date to restore files from (format: `YYYY-MM-DD`).
- `-s`, `--single-file`: Restore a single file from the recycle bin for a given date.
- `-h`, `--help`: Display the help message.

## Upcoming Features

- **File Monitoring**: Use `fsnotify` to track changes (creation, modification, or deletion) on specified files.
- **Automated Backups**: Automatically back up modified files to a designated directory.

## Uninstallation

Remove Cbin CLI with this command:

```bash
curl -sSL https://github.com/Toymakerftw/recycler/raw/refs/heads/wip/agent/bin/uninstall.sh | sudo bash
```

## Building from Source

If you prefer to build Cbin from source, follow these steps:

1.  **Clone the repository:**
    
    Bash
    
    ```
    git clone https://github.com/toymakerftw/recycler.git]
    cd cbin
    ```
    
2.  **Build the binaries:**
    
    Bash
    
    ```
    go build -o agent/bin/cbin agent/src/main.go
    go build -o agent/bin/health agent/health-checker/health.go
    ```

---

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for more details.

---

## Contributing

We welcome contributions! Feel free to open issues or submit pull requests to improve the project.

---

## Author

Developed by Anandhraman. 🚀

--- 

Let me know if you'd like further tweaks!
