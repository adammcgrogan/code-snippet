# Code Snippet CLI 📸

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)

A command-line tool that turns your code into syntax-highlighted images suitable for sharing.

It automatically detects the programming language, applies a "Dracula" dark theme, and wraps the code in a macOS-style window frame with drop shadows.

## ✨ Features

* **Syntax Highlighting:** Automatically detects languages (Go, Python, JSON, etc.) and applies colorful syntax highlighting.
* **Clipboard:** `--copy` flag to copy the snippet directly to the clipboard for easy sharing.
* **Window Controls:** Renders a clean "window" interface with traffic light buttons.
* **Line Extraction:** Extract and render only specific lines from a large file.
* **Smart Input:** Accepts file paths arguments OR piped input from stdin.

## 📦 Installation

**Prerequisites:** Go 1.21+

1.  **Clone the repository:**
    ```bash
    git clone [https://github.com/adammcgrogan/code-snippet.git](https://github.com/adammcgrogan/code-snippet.git)
    cd code-snippet
    ```

2.  **Build the binary:**
    ```bash
    go build -o code-snippet main.go
    ```

## 🚀 Usage

### Basic Usage
```bash
# Entire file
code-snippet main.go

# Entire file & copy to clipboard
code-snippet main.go --copy

# Lines 10-20
code-snippet main.go -l 10-20

# Piped in input
echo 'fmt.Println("Hello World")' | code-snippet
```