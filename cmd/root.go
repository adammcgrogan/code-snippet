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

// Global flags
var lineRange string
var copyToClipboard bool

// rootCmd represents the main command for the CLI.
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

		if copyToClipboard {
			if err := clipboard.Init(); err != nil {
				fmt.Printf("⚠️  Warning: Failed to initialize clipboard: %v\n", err)
			}
		}

		startLine := 1

		if lineRange != "" {
			var err error
			code, startLine, err = extractLines(code, lineRange)
			if err != nil {
				fmt.Printf("Error processing lines: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✂️  Extracted lines %s\n", lineRange)
		}

		fmt.Printf("🎨 Rendering '%s'...\n", filename)

		err := generateImage(code, filename, startLine)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

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

// extractLines parses a string range (start-end) and returns the subset of code lines.
func extractLines(code string, rangeStr string) (string, int, error) {
	lines := strings.Split(code, "\n")
	totalLines := len(lines)

	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid format. Use start-end (e.g. 10-20)")
	}

	start, err1 := strconv.Atoi(parts[0])
	end, err2 := strconv.Atoi(parts[1])

	if err1 != nil || err2 != nil {
		return "", 0, fmt.Errorf("line numbers must be integers")
	}

	if start < 1 {
		start = 1
	}
	if end > totalLines {
		end = totalLines
	}
	if start > end {
		return "", 0, fmt.Errorf("start line cannot be greater than end line")
	}

	subset := lines[start-1 : end]

	return strings.Join(subset, "\n"), start, nil
}

// generateImage handles the core logic of syntax highlighting and drawing.
func generateImage(code, filename string, startLine int) error {
	code = strings.ReplaceAll(code, "\t", "    ")
	const fontSize = 24.0
	const lineSpacing = 1.5
	const padding = 40.0

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

	fontBytes, _ := fontFS.ReadFile("fonts/font.ttf")
	tempFont, _ := os.CreateTemp("", "code-font-*.ttf")
	tempFont.Write(fontBytes)
	tempFont.Close()
	defer os.Remove(tempFont.Name())

	// Measure dimensions
	dummyDc := gg.NewContext(1, 1)
	dummyDc.LoadFontFace(tempFont.Name(), fontSize)

	lines := strings.Split(code, "\n")
	lineCount := len(lines)

	maxLineNumStr := fmt.Sprintf("%d", startLine+lineCount)
	gutterWidth, _ := dummyDc.MeasureString(maxLineNumStr)
	gutterWidth += 30

	imgHeight := int((float64(lineCount) * fontSize * lineSpacing) + (padding * 3))

	maxLineWidth := 0.0
	for _, line := range lines {
		w, _ := dummyDc.MeasureString(line)
		if w > maxLineWidth {
			maxLineWidth = w
		}
	}

	imgWidth := int(padding + gutterWidth + maxLineWidth + padding)
	if imgWidth < 600 {
		imgWidth = 600
	}

	// Initialize Canvas
	dc := gg.NewContext(imgWidth, imgHeight)
	dc.SetHexColor("#282a36")
	dc.Clear()

	if err := dc.LoadFontFace(tempFont.Name(), fontSize); err != nil {
		return err
	}

	drawWindowUI(dc, imgWidth, filename)

	xCodeStart := padding + gutterWidth
	y := padding + 40.0
	currentLineNum := startLine
	currentX := xCodeStart

	// Draw first line number
	dc.SetHexColor("#6272a4")
	dc.DrawStringAnchored(fmt.Sprintf("%d", currentLineNum), padding+gutterWidth-15, y, 1, 0)

	for _, token := range iterator.Tokens() {
		if token.Value == "\n" {
			currentX = xCodeStart
			y += fontSize * lineSpacing
			currentLineNum++

			if currentLineNum < startLine+lineCount {
				dc.SetHexColor("#6272a4")
				dc.DrawStringAnchored(fmt.Sprintf("%d", currentLineNum), padding+gutterWidth-15, y, 1, 0)
			}
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

		dc.DrawString(token.Value, currentX, y)
		w, _ := dc.MeasureString(token.Value)
		currentX += w
	}

	if copyToClipboard {
		var buf bytes.Buffer
		if err := dc.EncodePNG(&buf); err != nil {
			return err
		}
		clipboard.Write(clipboard.FmtImage, buf.Bytes())
	}

	return dc.SavePNG("snippet.png")
}

// drawWindowUI draws the window frame buttons and filename title.
func drawWindowUI(dc *gg.Context, width int, title string) {
	// Traffic lights
	dc.SetHexColor("#ff5f56")
	dc.DrawCircle(30, 30, 8)
	dc.Fill()
	dc.SetHexColor("#ffbd2e")
	dc.DrawCircle(55, 30, 8)
	dc.Fill()
	dc.SetHexColor("#27c93f")
	dc.DrawCircle(80, 30, 8)
	dc.Fill()

	// Title
	dc.SetHexColor("#6272a4")
	dc.DrawStringAnchored(title, float64(width)/2, 30, 0.5, 0.4)
}

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
