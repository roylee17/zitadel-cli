// Package output provides flexible output formatting for CLI commands.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"
)

// Format represents the output format.
type Format string

// Output format constants.
const (
	FormatTable      Format = "table"
	FormatWide       Format = "wide"
	FormatJSON       Format = "json"
	FormatYAML       Format = "yaml"
	FormatGoTemplate Format = "go-template"
	FormatName       Format = "name"
)

// ParseFormat parses a format string.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return FormatTable, nil
	case "wide":
		return FormatWide, nil
	case "json":
		return FormatJSON, nil
	case "yaml":
		return FormatYAML, nil
	case "name":
		return FormatName, nil
	default:
		if strings.HasPrefix(s, "go-template=") {
			return FormatGoTemplate, nil
		}
		return "", fmt.Errorf("unknown format: %s", s)
	}
}

// Printer handles output formatting.
type Printer struct {
	out      io.Writer
	format   Format
	template string
	noColor  bool
}

// NewPrinter creates a new printer with the specified format.
func NewPrinter(format Format, templateStr string) *Printer {
	noColor := os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
	if noColor {
		color.NoColor = true
	}

	return &Printer{
		out:      os.Stdout,
		format:   format,
		template: templateStr,
		noColor:  noColor,
	}
}

// SetOutput sets the output writer.
func (p *Printer) SetOutput(w io.Writer) {
	p.out = w
}

// PrintTable prints data as a table with generic item support.
func PrintTable[T any](p *Printer, headers []string, items []T, rowFunc func(T) []string) error {
	rows := make([][]string, len(items))
	for i, item := range items {
		rows[i] = rowFunc(item)
	}
	return p.PrintTableRows(headers, rows)
}

// PrintTableRows prints data as a table from pre-formatted rows.
func (p *Printer) PrintTableRows(headers []string, rows [][]string) error {
	switch p.format {
	case FormatJSON:
		return p.printTableAsJSON(headers, rows)
	case FormatYAML:
		return p.printTableAsYAML(headers, rows)
	case FormatGoTemplate:
		return p.printTableAsTemplate(headers, rows)
	case FormatName:
		return p.printNames(rows)
	case FormatTable, FormatWide:
		return p.printTableFormatted(headers, rows)
	}
	return p.printTableFormatted(headers, rows)
}

// PrintTable prints data as a table (legacy method for compatibility).
func (p *Printer) PrintTable(_ []string, items, _ interface{}) error {
	// Use reflection to handle generic types
	// This is a compatibility shim - prefer PrintTableRows for direct use
	switch p.format {
	case FormatJSON:
		return p.PrintJSON(items)
	case FormatYAML:
		return p.PrintYAML(items)
	case FormatTable, FormatWide, FormatGoTemplate, FormatName:
		return p.PrintJSON(items)
	}
	return p.PrintJSON(items)
}

func (p *Printer) printTableFormatted(headers []string, rows [][]string) error {
	table := tablewriter.NewWriter(p.out)
	table.SetHeader(headers)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)
	table.AppendBulk(rows)
	table.Render()
	return nil
}

func (p *Printer) printTableAsJSON(headers []string, rows [][]string) error {
	result := make([]map[string]string, len(rows))
	for i, row := range rows {
		item := make(map[string]string)
		for j, h := range headers {
			if j < len(row) {
				item[strings.ToLower(h)] = row[j]
			}
		}
		result[i] = item
	}
	return p.PrintJSON(result)
}

func (p *Printer) printTableAsYAML(headers []string, rows [][]string) error {
	result := make([]map[string]string, len(rows))
	for i, row := range rows {
		item := make(map[string]string)
		for j, h := range headers {
			if j < len(row) {
				item[strings.ToLower(h)] = row[j]
			}
		}
		result[i] = item
	}
	return p.PrintYAML(result)
}

func (p *Printer) printTableAsTemplate(headers []string, rows [][]string) error {
	tmpl, err := template.New("output").Parse(p.template)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	for _, row := range rows {
		item := make(map[string]string)
		for j, h := range headers {
			if j < len(row) {
				item[strings.ToLower(h)] = row[j]
			}
		}
		if err := tmpl.Execute(p.out, item); err != nil {
			return fmt.Errorf("execute template: %w", err)
		}
		_, _ = fmt.Fprintln(p.out)
	}
	return nil
}

func (p *Printer) printNames(rows [][]string) error {
	for _, row := range rows {
		if len(row) > 0 {
			_, _ = fmt.Fprintln(p.out, row[0])
		}
	}
	return nil
}

// PrintJSON prints data as JSON.
func (p *Printer) PrintJSON(v interface{}) error {
	enc := json.NewEncoder(p.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// PrintYAML prints data as YAML.
func (p *Printer) PrintYAML(v interface{}) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = p.out.Write(data)
	return err
}

// Print prints data in the configured format.
func (p *Printer) Print(v interface{}) error {
	switch p.format {
	case FormatJSON:
		return p.PrintJSON(v)
	case FormatYAML:
		return p.PrintYAML(v)
	case FormatGoTemplate:
		return p.printTemplate(v)
	case FormatTable, FormatWide, FormatName:
		return p.PrintJSON(v)
	}
	return p.PrintJSON(v)
}

func (p *Printer) printTemplate(v interface{}) error {
	tmpl, err := template.New("output").Parse(p.template)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	return tmpl.Execute(p.out, v)
}

// Success prints a success message.
func (p *Printer) Success(format string, args ...interface{}) {
	if p.noColor {
		_, _ = fmt.Fprintf(os.Stderr, "✓ "+format+"\n", args...)
	} else {
		_, _ = color.New(color.FgGreen).Fprintf(os.Stderr, "✓ "+format+"\n", args...)
	}
}

// Error prints an error message.
func (p *Printer) Error(format string, args ...interface{}) {
	if p.noColor {
		_, _ = fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	} else {
		_, _ = color.New(color.FgRed).Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	}
}

// Warning prints a warning message.
func (p *Printer) Warning(format string, args ...interface{}) {
	if p.noColor {
		_, _ = fmt.Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
	} else {
		_, _ = color.New(color.FgYellow).Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
	}
}

// Info prints an info message.
func (p *Printer) Info(format string, args ...interface{}) {
	if p.noColor {
		_, _ = fmt.Fprintf(os.Stderr, "ℹ "+format+"\n", args...)
	} else {
		_, _ = color.New(color.FgCyan).Fprintf(os.Stderr, "ℹ "+format+"\n", args...)
	}
}

// Sprintf returns a formatted string.
func Sprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// Bold returns bold text.
func Bold(s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return color.New(color.Bold).Sprint(s)
}

// Green returns green text.
func Green(s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return color.GreenString(s)
}

// Red returns red text.
func Red(s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return color.RedString(s)
}

// Yellow returns yellow text.
func Yellow(s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return color.YellowString(s)
}

// Cyan returns cyan text.
func Cyan(s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return color.CyanString(s)
}

// RenderTemplate renders a Go template with the given data.
func RenderTemplate(tmplStr string, data interface{}) (string, error) {
	tmpl, err := template.New("output").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// PrintObject prints an object in the configured format.
func (p *Printer) PrintObject(v interface{}) error {
	return p.Print(v)
}

// PrintKeyValue prints a key-value map in a readable format.
func (p *Printer) PrintKeyValue(kv map[string]string) {
	switch p.format {
	case FormatJSON:
		_ = p.PrintJSON(kv)
	case FormatYAML:
		_ = p.PrintYAML(kv)
	case FormatTable, FormatWide, FormatGoTemplate, FormatName:
		p.printKeyValueFormatted(kv)
	}
}

func (p *Printer) printKeyValueFormatted(kv map[string]string) {
	maxKeyLen := 0
	for k := range kv {
		if len(k) > maxKeyLen {
			maxKeyLen = len(k)
		}
	}
	for k, v := range kv {
		if v != "" {
			_, _ = fmt.Fprintf(p.out, "  %-*s  %s\n", maxKeyLen, k+":", v)
		}
	}
}

// Header prints a section header.
func (p *Printer) Header(s string) {
	if p.noColor {
		_, _ = fmt.Fprintf(os.Stderr, "=== %s ===\n", s)
	} else {
		_, _ = color.New(color.Bold).Fprintf(os.Stderr, "=== %s ===\n", s)
	}
}
