package localparser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Extract reads a file from the given reader and extracts plain text based on
// the file extension. This is the fallback parser used when MinerU is not
// available. It supports DOCX, PDF, and text-based formats.
func Extract(r io.Reader, filename string) (string, error) {
	ext := strings.ToLower(filename)
	if i := strings.LastIndex(ext, "."); i >= 0 {
		ext = ext[i:]
	}

	switch ext {
	case ".docx":
		return extractDOCX(r)
	case ".pdf":
		return extractPDF(r)
	case ".txt", ".md", ".markdown", ".csv", ".json", ".log":
		b, err := io.ReadAll(r)
		if err != nil {
			return "", err
		}
		return string(b), nil
	default:
		b, err := io.ReadAll(r)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

// extractDOCX parses a .docx file (ZIP archive) and extracts text from
// word/document.xml. It walks <w:p> paragraphs and collects <w:t> text runs.
func extractDOCX(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read docx: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("open docx zip: %w", err)
	}

	var docXML []byte
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("open document.xml: %w", err)
			}
			docXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return "", fmt.Errorf("read document.xml: %w", err)
			}
			break
		}
	}
	if docXML == nil {
		return "", fmt.Errorf("word/document.xml not found in docx")
	}

	return parseDocxXML(docXML)
}

// w:t is the text element, w:p is paragraph, w:br is break, w:tab is tab
type docxText struct {
	Text string `xml:",chardata"`
}
type docxParagraph struct{}
type docxTab struct{}
type docxBreak struct{}

func parseDocxXML(data []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var sb strings.Builder
	var inParagraph bool

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse docx xml: %w", err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			local := v.Name.Local
			switch local {
			case "p":
				inParagraph = true
			case "t":
				var txt docxText
				if err := dec.DecodeElement(&txt, &v); err == nil {
					sb.WriteString(txt.Text)
				}
			case "tab":
				sb.WriteString("\t")
			case "br":
				sb.WriteString("\n")
			}
		case xml.EndElement:
			if v.Name.Local == "p" && inParagraph {
				sb.WriteString("\n")
				inParagraph = false
			}
		}
	}
	return sb.String(), nil
}

// extractPDF uses the ledongthuc/pdf library to extract plain text from all
// pages of a PDF file.
func extractPDF(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read pdf: %w", err)
	}
	reader := bytes.NewReader(b)
	pdfReader, err := pdf.NewReader(reader, int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}

	var sb strings.Builder
	numPages := pdfReader.NumPage()
	for i := 1; i <= numPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}
	return sb.String(), nil
}
