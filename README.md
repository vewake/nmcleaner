#  nmcleaner

A fast, interactive terminal UI tool to find and delete `node_modules` folders recursively from your directories, helping you free up disk space easily.

## Features

- **Interactive TUI**: Navigate and select folders using standard CLI bindings (Vim keys supported).
- **Fast Scanning**: Quickly calculates the total size of your `node_modules` directories.
- **Tree View**: Visualize nested directory structures clearly.
- **Bulk Delete**: Select and delete multiple `node_modules` directories in one go.

## Installation

### Method 1: Using `go install` (Recommended)

If you have Go installed (1.20+), you can easily install the tool directly using `go install`. This is the easiest way to get the latest version.

```bash
go install github.com/vewake/nmcleaner@latest
```
*Make sure your `$(go env GOPATH)/bin` directory is in your system's `$PATH`.*

### Method 2: Pre-compiled Binaries

1. Go to the [Releases page](https://github.com/vewake/nmcleaner/releases/latest).
2. Download the archive corresponding to your OS and CPU architecture.
3. Extract the archive.

**macOS / Linux:**
```bash
chmod +x nmcleaner
./nmcleaner
```

**Windows (PowerShell):**
```powershell
.\nmcleaner.exe
```

## Usage

Simply run the command in the directory you want to clean. If installed via `go install`:

```bash
nmcleaner
```
*(or `./nmcleaner` if you downloaded the binary manually)*

### Controls

- <kbd>↑</kbd> <kbd>↓</kbd> or <kbd>k</kbd> / <kbd>j</kbd>: Move cursor
- <kbd>Space</kbd>: Select/Deselect current item
- <kbd>a</kbd>: Select/Deselect all items
- <kbd>Tab</kbd> or <kbd>e</kbd>: Expand/Collapse tree structure
- <kbd>Enter</kbd>: Delete selected items permanently
- <kbd>q</kbd> or <kbd>Ctrl+C</kbd>: Quit application

## Build from Source

To build the project manually from the source code:

```bash
git clone https://github.com/vewake/nmcleaner.git
cd nmcleaner
go build -o nmcleaner .
./nmcleaner
```

## License

[MIT](LICENSE)
