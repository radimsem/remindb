package doctor

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"

	glyphPass = "✓"
	glyphWarn = "⚠"
	glyphFail = "✗"
)

func (r Report) WriteText(w io.Writer, color bool) error {
	for _, c := range r.Checks {
		glyph := paintGlyph(c.Status, color)
		line := fmt.Sprintf("%s %-20s %s", glyph, c.Name, c.Detail)

		if c.FixApplied {
			line += "  (fixed)"
		}
		if c.FixError != "" {
			line += "  (fix error: " + c.FixError + ")"
		}

		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func paintGlyph(status string, color bool) string {
	glyph, code := glyphAndColor(status)
	if !color {
		return glyph
	}
	return code + glyph + ansiReset
}

func glyphAndColor(status string) (glyph, code string) {
	switch status {
	case "pass":
		return glyphPass, ansiGreen
	case "warn":
		return glyphWarn, ansiYellow
	case "fail":
		return glyphFail, ansiRed
	default:
		return "?", ""
	}
}
