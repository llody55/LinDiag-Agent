package output

import (
"fmt"
"strings"
)

const (
Red     = "\033[31m"
Green   = "\033[32m"
Yellow  = "\033[33m"
Blue    = "\033[34m"
Cyan    = "\033[36m"
Bold    = "\033[1m"
Reset   = "\033[0m"
Underline = "\033[4m"
)

type Table struct {
Headers []string
Rows    [][]string
}

func (t *Table) AddRow(row []string) {
t.Rows = append(t.Rows, row)
}

func (t *Table) Render() string {
if len(t.Headers) == 0 {
return ""
}

colWidths := make([]int, len(t.Headers))

for i, header := range t.Headers {
colWidths[i] = len(header)
}

for _, row := range t.Rows {
for i, cell := range row {
if len(cell) > colWidths[i] {
colWidths[i] = len(cell)
}
}
}

var sb strings.Builder

border := "+"
for _, w := range colWidths {
border += strings.Repeat("-", w+2) + "+"
}

sb.WriteString(border + "\n")

sb.WriteString("|")
for i, header := range t.Headers {
sb.WriteString(fmt.Sprintf(" %-*s |", colWidths[i], header))
}
sb.WriteString("\n")

sb.WriteString(border + "\n")

for _, row := range t.Rows {
sb.WriteString("|")
for i, cell := range row {
sb.WriteString(fmt.Sprintf(" %-*s |", colWidths[i], cell))
}
sb.WriteString("\n")
}

sb.WriteString(border + "\n")

return sb.String()
}

func PrintTable(t *Table) {
fmt.Print(t.Render())
}

func Colorize(text, color string) string {
return color + text + Reset
}

func BoldText(text string) string {
return Bold + text + Reset
}

func SuccessMessage(msg string) {
fmt.Println(Green + "✅ " + msg + Reset)
}

func ErrorMessage(msg string) {
fmt.Println(Red + "❌ " + msg + Reset)
}

func WarningMessage(msg string) {
fmt.Println(Yellow + "⚠️ " + msg + Reset)
}

func InfoMessage(msg string) {
fmt.Println(Cyan + "ℹ️ " + msg + Reset)
}

func SectionTitle(title string) {
fmt.Println("\n" + Bold + Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
fmt.Println(Bold + Cyan + "│" + Reset + " " + BoldText(title))
fmt.Println(Bold + Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
}

func CommandBox(cmd string, explanation string) {
fmt.Println("\n" + Cyan + "┌─────────────────────────────────────────────────────────────" + Reset)
fmt.Println(Cyan + "│" + Reset + " 🔍 " + BoldText("执行命令"))
fmt.Println(Cyan + "├─────────────────────────────────────────────────────────────" + Reset)
fmt.Println(Cyan + "│" + Reset + " 命令: " + Yellow + cmd + Reset)
fmt.Println(Cyan + "│" + Reset + " 说明: " + explanation)
fmt.Println(Cyan + "└─────────────────────────────────────────────────────────────" + Reset)
}

func ResultBox(success bool, output string) {
color := Green
prefix := "✅"
if !success {
color = Red
prefix = "❌"
}

fmt.Println(color + "┌─────────────────────────────────────────────────────────────" + Reset)
fmt.Println(color + "│" + Reset + " " + prefix + " " + BoldText("执行结果"))
fmt.Println(color + "└─────────────────────────────────────────────────────────────" + Reset)
fmt.Println(output)
}

func Header(title string) {
fmt.Println("\n" + Bold + Yellow + "╔══════════════════════════════════════════════════════════════════════════╗" + Reset)
fmt.Println(Bold + Yellow + "║" + Reset + " " + strings.Repeat(" ", (80-len(title)-2)/2) + BoldText(title) + strings.Repeat(" ", (80-len(title)-2)/2) + " " + Bold + Yellow + "║" + Reset)
fmt.Println(Bold + Yellow + "╚══════════════════════════════════════════════════════════════════════════╝" + Reset)
}

func SubHeader(title string) {
fmt.Println("\n" + Bold + Blue + "├─ " + title + " ──────────────────────────────────────────────────────────────" + Reset)
}
