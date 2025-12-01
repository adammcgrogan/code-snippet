package cmd

import (
	"bytes"
	"embed"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/fogleman/gg"
	"github.com/spf13/cobra"
	"golang.design/x/clipboard"
)

//go:embed fonts/font.ttf
var fontFS embed.FS

// CLI Flags
var lineRange string
var copyToClipboard bool

// rootCmd defines the primary CLI command application.
// It handles argument parsing, input validation, and triggers the rendering pipeline.
var rootCmd = &cobra.Command{
	Use:   "code-snippet [file]",
	Short: "Turn code into a beautiful image",
	Example: `  code-snippet main.go
  code-snippet main.go -l 10-20
  code-snippet main.go --copy`,
	Run: func(cmd *cobra.Command, args []string) {
		code, filename := readInput(args)

		if code == "" {
			fmt.Println("Error: No input provided.")
			return
		}

		// Initialize clipboard access if the flag is set.
		// This requires OS-level access and may fail on headless environments.
		if copyToClipboard {
			if err := clipboard.Init(); err != nil {
				fmt.Printf("⚠️  Warning: Failed to initialize clipboard: %v\n", err)
			}
		}

		// Process the --lines flag if provided.
		// This extracts the specific subset of code requested by the user.
		if lineRange != "" {
			var err error
			code, err = extractLines(code, lineRange)
			if err != nil {
				fmt.Printf("Error processing lines: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✂️  Extracted lines %s\n", lineRange)
		}

		fmt.Printf("🎨 Rendering '%s'...\n", filename)
		err := generateImage(code, filename)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Final output message depends on flags
		if copyToClipboard {
			fmt.Println("✅ Saved to snippet.png AND copied to clipboard!")
		} else {
			fmt.Println("✅ Saved to snippet.png")
		}
	},
}

func init() {
	rootCmd.Flags().StringVarP(&lineRange, "lines", "l", "", "Line range to render (e.g. 10-20)")
	rootCmd.Flags().BoolVarP(&copyToClipboard, "copy", "c", false, "Copy image to system clipboard")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// extractLines parses the line range string (e.g., "10-20") and returns
// a subset of the code string. It handles bounds checking to ensure
// the range is valid within the file.
func extractLines(code string, rangeStr string) (string, error) {
	lines := strings.Split(code, "\n")
	totalLines := len(lines)

	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid format. Use start-end (e.g. 10-20)")
	}

	start, err1 := strconv.Atoi(parts[0])
	end, err2 := strconv.Atoi(parts[1])

	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("line numbers must be integers")
	}

	if start < 1 {
		start = 1
	}
	if end > totalLines {
		end = totalLines
	}
	if start > end {
		return "", fmt.Errorf("start line cannot be greater than end line")
	}

	subset := lines[start-1 : end]

	return strings.Join(subset, "\n"), nil
}

// generateImage handles the core rendering logic.
// It performs syntax highlighting using Chroma, creates an image context using gg,
// calculates dynamic sizing based on text content, and saves the final PNG.
func generateImage(code, filename string) error {
	// Pre-process code string to replace tabs with spaces for consistent rendering.
	code = strings.ReplaceAll(code, "\t", "    ")
	const fontSize = 24.0
	const lineSpacing = 1.5
	const padding = 40.0

	// Determine the Lexer (language syntax) based on filename or content analysis.
	// Defaults to Go if detection fails.
	var lexer chroma.Lexer
	if filename != "Stdin" {
		lexer = lexers.Match(filename)
	}
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Get("go")
	}

	style := styles.Get("dracula")
	if style == nil {
		style = styles.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return err
	}

	// Extract the embedded font file to a temporary location so the graphics library can load it.
	fontBytes, _ := fontFS.ReadFile("fonts/font.ttf")
	tempFont, _ := os.CreateTemp("", "code-font-*.ttf")
	tempFont.Write(fontBytes)
	tempFont.Close()
	defer os.Remove(tempFont.Name())

	// Perform a "Dry Run" to calculate the required image dimensions.
	// We iterate through lines to find the maximum width and total height.
	dummyDc := gg.NewContext(1, 1)
	dummyDc.LoadFontFace(tempFont.Name(), fontSize)

	lines := strings.Split(code, "\n")
	imgHeight := int((float64(len(lines)) * fontSize * lineSpacing) + (padding * 3))

	maxLineWidth := 0.0
	for _, line := range lines {
		w, _ := dummyDc.MeasureString(line)
		if w > maxLineWidth {
			maxLineWidth = w
		}
	}
	imgWidth := int(maxLineWidth + (padding * 2))
	if imgWidth < 600 {
		imgWidth = 600
	}

	// Initialize the real image context, draw background, and window controls.
	dc := gg.NewContext(imgWidth, imgHeight)
	dc.SetHexColor("#282a36")
	dc.Clear()
	drawWindowControls(dc)
	dc.LoadFontFace(tempFont.Name(), fontSize)

	x := padding
	y := padding + 40.0

	// Iterate over the syntax tokens, applying colors from the theme and drawing text.
	for _, token := range iterator.Tokens() {
		if token.Value == "\n" {
			x = padding
			y += fontSize * lineSpacing
			continue
		}

		entry := style.Get(token.Type)
		if entry.Colour.IsSet() {
			r := entry.Colour.Red()
			g := entry.Colour.Green()
			b := entry.Colour.Blue()
			dc.SetColor(color.RGBA{R: r, G: g, B: b, A: 255})
		} else {
			dc.SetHexColor("#f8f8f2")
		}

		dc.DrawString(token.Value, x, y)
		w, _ := dc.MeasureString(token.Value)
		x += w
	}

	// Logic for Clipboard vs Disk
	if copyToClipboard {
		// Encode the image to a memory buffer (RAM) instead of disk
		var buf bytes.Buffer
		if err := dc.EncodePNG(&buf); err != nil {
			return err
		}
		// Write the buffer bytes to the OS Clipboard
		clipboard.Write(clipboard.FmtImage, buf.Bytes())
	}

	// We always save the file as a backup
	return dc.SavePNG("snippet.png")
}

// drawWindowControls renders the macOS-style window buttons (Red, Yellow, Green)
// in the top-left corner of the image context.
func drawWindowControls(dc *gg.Context) {
	dc.SetHexColor("#ff5f56")
	dc.DrawCircle(30, 30, 8)
	dc.Fill()
	dc.SetHexColor("#ffbd2e")
	dc.DrawCircle(55, 30, 8)
	dc.Fill()
	dc.SetHexColor("#27c93f")
	dc.DrawCircle(80, 30, 8)
	dc.Fill()
}

// readInput determines the source of the code.
// It checks if a file argument was provided, otherwise it attempts to read from Stdin (pipes).
func readInput(args []string) (string, string) {
	if len(args) > 0 {
		content, err := os.ReadFile(args[0])
		if err != nil {
			return "", ""
		}
		return string(content), filepath.Base(args[0])
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		bytes, _ := io.ReadAll(os.Stdin)
		return string(bytes), "Stdin"
	}
	return "", ""
}
