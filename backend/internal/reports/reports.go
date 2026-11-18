package reports

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"
)

// SanitizeCSVCell prevents spreadsheet formula injection (CWE-1236) by prefixing cells
// starting with '=', '+', '-', '@', '\t', or '\r' with a single quote.
func SanitizeCSVCell(v string) string {
	if v == "" {
		return ""
	}
	switch v[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + v
	default:
		return v
	}
}

// ExportCSV renders headers and rows as RFC 4180 CSV with formula-injection protection.
func ExportCSV(headers []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	safeHeaders := make([]string, len(headers))
	for i, h := range headers {
		safeHeaders[i] = SanitizeCSVCell(h)
	}
	if err := w.Write(safeHeaders); err != nil {
		return nil, err
	}

	for _, row := range rows {
		safeRow := make([]string, len(row))
		for i, cell := range row {
			safeRow[i] = SanitizeCSVCell(cell)
		}
		if err := w.Write(safeRow); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ExportJSON renders structured report payloads cleanly.
func ExportJSON(payload any) ([]byte, error) {
	return json.MarshalIndent(payload, "", "  ")
}

// ExportPrintableHTML generates a clean, self-contained print-friendly HTML report suitable for browser PDF export.
func ExportPrintableHTML(title, subtitle string, headers []string, rows [][]string) []byte {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</title><style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;color:#0f172a;margin:32px;line-height:1.5;}
h1{margin:0 0 4px 0;font-size:22px;color:#0b1220;}
.meta{color:#475569;font-size:13px;margin-bottom:20px;}
table{width:100%;border-collapse:collapse;font-size:13px;}
th,td{border:1px solid #cbd5e1;padding:8px 10px;text-align:left;}
th{background:#f1f5f9;font-weight:600;}
tr:nth-child(even){background:#f8fafc;}
@media print{body{margin:12mm;}button{display:none;}}
</style></head><body>`)
	b.WriteString("<h1>" + html.EscapeString(title) + "</h1>")
	b.WriteString(fmt.Sprintf(`<div class="meta">%s &middot; Generated %s UTC</div>`,
		html.EscapeString(subtitle), time.Now().UTC().Format("2006-01-02 15:04:05")))
	b.WriteString("<table><thead><tr>")
	for _, h := range headers {
		b.WriteString("<th>" + html.EscapeString(h) + "</th>")
	}
	b.WriteString("</tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, cell := range row {
			b.WriteString("<td>" + html.EscapeString(cell) + "</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table></body></html>")
	return []byte(b.String())
}
