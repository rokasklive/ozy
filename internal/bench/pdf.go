package bench

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Minimal pure-Go PDF 1.4 writer and reader (D6). The writer emits uncompressed
// content streams with base-14 Helvetica and renders markdown-ish input as
// headings, paragraphs, and lists; the reader extracts text and metadata from
// exactly that class of PDF, which is what the pdf-toolkit and the grader use.
// Rich tables and images are out of scope.

const (
	pdfPageWidth  = 612.0 // US Letter, points
	pdfPageHeight = 792.0
	pdfMargin     = 72.0
)

// PDFMeta is the document metadata carried in the Info dictionary.
type PDFMeta struct {
	Title  string
	Author string
}

// renderedLine is one positioned line of text: its content, font size, and left
// indent (points from the margin).
type renderedLine struct {
	text   string
	size   float64
	indent float64
}

// BuildPDF renders markdown-ish body text into a PDF document. Headings (#, ##,
// ###), list items (-, *), and blank-line-separated paragraphs are recognized;
// everything else is a paragraph. Text is transliterated to WinAnsi so
// non-Latin diacritics cannot corrupt the encoding.
func BuildPDF(meta PDFMeta, body string) []byte {
	lines := layoutMarkdown(meta.Title, body)
	pages := paginate(lines)
	return assemblePDF(meta, pages)
}

// layoutMarkdown turns markdown-ish text into a flat list of rendered lines
// (word-wrapped), optionally prefixed with a title heading.
func layoutMarkdown(title, body string) []renderedLine {
	var out []renderedLine
	if title != "" {
		out = append(out, wrap(title, 20, 0)...)
		out = append(out, renderedLine{text: "", size: 11})
	}
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			out = append(out, renderedLine{text: "", size: 11})
		case strings.HasPrefix(trimmed, "### "):
			out = append(out, wrap(trimmed[4:], 13, 0)...)
		case strings.HasPrefix(trimmed, "## "):
			out = append(out, wrap(trimmed[3:], 16, 0)...)
		case strings.HasPrefix(trimmed, "# "):
			out = append(out, wrap(trimmed[2:], 20, 0)...)
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			item := wrap("- "+trimmed[2:], 11, 12)
			out = append(out, item...)
		default:
			out = append(out, wrap(trimmed, 11, 0)...)
		}
	}
	return out
}

// wrap word-wraps text to the printable width for the given font size, emitting
// one renderedLine per visual line.
func wrap(text string, size, indent float64) []renderedLine {
	// Helvetica averages ~0.5em per glyph; approximate max chars per line.
	maxChars := int((pdfPageWidth - 2*pdfMargin - indent) / (size * 0.5))
	if maxChars < 8 {
		maxChars = 8
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []renderedLine{{text: "", size: size, indent: indent}}
	}
	var lines []renderedLine
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case len(cur)+1+len(w) <= maxChars:
			cur += " " + w
		default:
			lines = append(lines, renderedLine{text: cur, size: size, indent: indent})
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, renderedLine{text: cur, size: size, indent: indent})
	}
	return lines
}

// paginate splits rendered lines into pages by cumulative vertical space.
func paginate(lines []renderedLine) [][]renderedLine {
	var pages [][]renderedLine
	var cur []renderedLine
	y := pdfPageHeight - pdfMargin
	for _, ln := range lines {
		lh := ln.size * 1.4
		if y-lh < pdfMargin && len(cur) > 0 {
			pages = append(pages, cur)
			cur = nil
			y = pdfPageHeight - pdfMargin
		}
		cur = append(cur, ln)
		y -= lh
	}
	if len(cur) > 0 {
		pages = append(pages, cur)
	}
	if len(pages) == 0 {
		pages = [][]renderedLine{{}}
	}
	return pages
}

// contentStream renders one page's lines into a PDF content stream.
func contentStream(lines []renderedLine) []byte {
	var b bytes.Buffer
	y := pdfPageHeight - pdfMargin
	for _, ln := range lines {
		y -= ln.size * 1.4
		if ln.text == "" {
			continue
		}
		x := pdfMargin + ln.indent
		fmt.Fprintf(&b, "BT /F1 %.1f Tf %.1f %.1f Td (%s) Tj ET\n", ln.size, x, y, escapePDFText(ln.text))
	}
	return b.Bytes()
}

// assemblePDF writes the full PDF: catalog, pages, font, one page+content object
// pair per page, and an Info dictionary, with a correct xref table.
func assemblePDF(meta PDFMeta, pages [][]renderedLine) []byte {
	// Object numbers: 1 catalog, 2 pages, 3 font, then per page a page obj and a
	// content obj, then Info last.
	pageObjNum := func(i int) int { return 4 + 2*i }
	contentObjNum := func(i int) int { return 5 + 2*i }
	infoNum := 4 + 2*len(pages)

	type obj struct {
		num  int
		body []byte
	}
	var objs []obj

	// Catalog + Pages.
	var kids strings.Builder
	for i := range pages {
		fmt.Fprintf(&kids, "%d 0 R ", pageObjNum(i))
	}
	objs = append(objs,
		obj{1, []byte("<< /Type /Catalog /Pages 2 0 R >>")},
		obj{2, []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.TrimSpace(kids.String()), len(pages)))},
		obj{3, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")},
	)

	for i, page := range pages {
		stream := contentStream(page)
		pageBody := fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>",
			pdfPageWidth, pdfPageHeight, contentObjNum(i))
		objs = append(objs, obj{pageObjNum(i), []byte(pageBody)})

		var sb bytes.Buffer
		fmt.Fprintf(&sb, "<< /Length %d >>\nstream\n", len(stream))
		sb.Write(stream)
		sb.WriteString("endstream")
		objs = append(objs, obj{contentObjNum(i), sb.Bytes()})
	}

	info := fmt.Sprintf("<< /Title (%s) /Author (%s) /Producer (ozy-bench pdf-toolkit) /CreationDate (D:%s) >>",
		escapePDFText(meta.Title), escapePDFText(meta.Author), time.Now().UTC().Format("20060102150405Z"))
	objs = append(objs, obj{infoNum, []byte(info)})

	// Serialize with byte offsets for the xref table.
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make(map[int]int, len(objs))
	for _, o := range objs {
		offsets[o.num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", o.num)
		buf.Write(o.body)
		buf.WriteString("\nendobj\n")
	}

	xrefOffset := buf.Len()
	size := infoNum + 1
	fmt.Fprintf(&buf, "xref\n0 %d\n", size)
	buf.WriteString("0000000000 65535 f \n")
	for n := 1; n < size; n++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[n])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		size, infoNum, xrefOffset)
	return buf.Bytes()
}

// winAnsiTranslit maps common non-WinAnsi Latin letters (notably Lithuanian
// diacritics) to ASCII so the WinAnsi-only writer never emits an unencodable
// byte. Runes still outside WinAnsi after this map are dropped.
var winAnsiTranslit = map[rune]string{
	'ą': "a", 'č': "c", 'ę': "e", 'ė': "e", 'į': "i", 'š': "s", 'ų': "u", 'ū': "u", 'ž': "z",
	'Ą': "A", 'Č': "C", 'Ę': "E", 'Ė': "E", 'Į': "I", 'Š': "S", 'Ų': "U", 'Ū': "U", 'Ž': "Z",
	'“': "\"", '”': "\"", '‘': "'", '’': "'", '–': "-", '—': "-", '…': "...",
}

// escapePDFText transliterates to WinAnsi and escapes the PDF string delimiters.
func escapePDFText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if repl, ok := winAnsiTranslit[r]; ok {
			b.WriteString(escapeRunes(repl))
			continue
		}
		switch r {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			if r >= 0x20 && r <= 0xFF {
				b.WriteRune(r)
			}
			// runes outside WinAnsi and not in the translit map are dropped.
		}
	}
	return b.String()
}

func escapeRunes(s string) string {
	return strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(s)
}

// ---------------------------------------------------------------------------
// Reader
// ---------------------------------------------------------------------------

var (
	pdfStreamRe = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	pdfMetaRe   = func(key string) *regexp.Regexp {
		return regexp.MustCompile(`/` + key + `\s*\((.*?[^\\]|)\)`)
	}
)

// ExtractPDFText recovers the text from a PDF produced by BuildPDF: it reads
// every uncompressed content stream and concatenates the literal strings (one
// per line), unescaped.
func ExtractPDFText(data []byte) string {
	var out []string
	for _, m := range pdfStreamRe.FindAllSubmatch(data, -1) {
		out = append(out, extractStrings(m[1])...)
	}
	return strings.Join(out, "\n")
}

// extractStrings pulls the parenthesized literal strings from a content stream
// in order, honoring PDF backslash escapes.
func extractStrings(stream []byte) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	esc := false
	for i := 0; i < len(stream); i++ {
		c := stream[i]
		if depth == 0 {
			if c == '(' {
				depth = 1
				cur.Reset()
			}
			continue
		}
		switch {
		case esc:
			cur.WriteByte(c)
			esc = false
		case c == '\\':
			esc = true
		case c == '(':
			depth++
			cur.WriteByte(c)
		case c == ')':
			depth--
			if depth == 0 {
				out = append(out, cur.String())
			} else {
				cur.WriteByte(c)
			}
		default:
			cur.WriteByte(c)
		}
	}
	return out
}

// PDFMetadata extracts the Info-dictionary Title and Author from a PDF produced
// by BuildPDF.
func PDFMetadata(data []byte) PDFMeta {
	get := func(key string) string {
		if m := pdfMetaRe(key).FindSubmatch(data); m != nil {
			return unescapePDFString(string(m[1]))
		}
		return ""
	}
	return PDFMeta{Title: get("Title"), Author: get("Author")}
}

func unescapePDFString(s string) string {
	return strings.NewReplacer("\\(", "(", "\\)", ")", "\\\\", "\\").Replace(s)
}

// IsPDF reports whether data begins with the PDF header.
func IsPDF(data []byte) bool {
	return bytes.HasPrefix(data, []byte("%PDF-"))
}
