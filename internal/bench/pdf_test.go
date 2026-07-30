package bench

import (
	"strings"
	"testing"
)

func TestBuildPDFRoundTrip(t *testing.T) {
	t.Parallel()

	body := "# Vilnius Weather Report\n\n" +
		"On 2024-07-15 the maximum temperature was 24.3 C.\n\n" +
		"## Facts\n\n" +
		"- Vilnius is the capital of Lithuania\n" +
		"- It sits on the Neris river\n"
	data := BuildPDF(PDFMeta{Title: "Weather Report", Author: "ozy-bench"}, body)

	if !IsPDF(data) {
		t.Fatal("output is not a PDF")
	}
	text := ExtractPDFText(data)
	for _, want := range []string{"Vilnius Weather Report", "24.3 C", "capital of Lithuania", "Neris river"} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text missing %q\n--- got ---\n%s", want, text)
		}
	}
}

func TestPDFMetadata(t *testing.T) {
	t.Parallel()

	data := BuildPDF(PDFMeta{Title: "My Title", Author: "Some Author"}, "body text")
	meta := PDFMetadata(data)
	if meta.Title != "My Title" {
		t.Errorf("title = %q, want %q", meta.Title, "My Title")
	}
	if meta.Author != "Some Author" {
		t.Errorf("author = %q, want %q", meta.Author, "Some Author")
	}
}

func TestPDFWinAnsiTransliteration(t *testing.T) {
	t.Parallel()

	// Lithuanian diacritics must transliterate to ASCII, never a raw non-WinAnsi
	// byte, and remain findable.
	data := BuildPDF(PDFMeta{Title: "Vilnius"}, "Čiurlionis in Vilnius, Lietuva")
	text := ExtractPDFText(data)
	if !strings.Contains(text, "Ciurlionis") {
		t.Errorf("transliteration failed: %q", text)
	}
	for _, r := range string(data) {
		if r > 0xFF {
			t.Errorf("PDF contains non-WinAnsi rune %q", r)
		}
	}
}

func TestPDFEscaping(t *testing.T) {
	t.Parallel()

	// Parens and backslashes in text must round-trip without breaking the PDF.
	data := BuildPDF(PDFMeta{Title: "t"}, "temp (max) was 24.3 C \\ high")
	text := ExtractPDFText(data)
	if !strings.Contains(text, "(max)") || !strings.Contains(text, "24.3 C") {
		t.Errorf("escaping round-trip failed: %q", text)
	}
}

func TestPDFPagination(t *testing.T) {
	t.Parallel()

	// Enough lines to force multiple pages; text from a late line must survive.
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("Line of body text number ")
		b.WriteString(strings.Repeat("x", 3))
		b.WriteString("\n\n")
	}
	b.WriteString("FINAL_MARKER_LINE\n")
	data := BuildPDF(PDFMeta{Title: "Long"}, b.String())
	if !strings.Contains(ExtractPDFText(data), "FINAL_MARKER_LINE") {
		t.Error("paginated content lost the final line")
	}
}
