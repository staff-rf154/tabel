package pdf

import (
	"bytes"
	"testing"
)

func TestPaperQR(t *testing.T) {
	document, err := PaperQR(PaperQRData{
		EmployeeName:   "Иванов Иван Иванович",
		EmployeeNumber: "E-0001",
		Position:       "Инженер",
		Token:          "11111111-1111-1111-1111-111111111111",
		ValidFrom:      "2026-10-19",
		ValidUntil:     "2026-11-19",
		Status:         "active",
		QRContent:      `{"token":"11111111-1111-1111-1111-111111111111","signature":"test","payload":{"full_name":"Иванов Иван Иванович"}}`,
		SupportPhone:   "+7 999 000-00-00",
	})
	if err != nil {
		t.Fatalf("PaperQR: %v", err)
	}
	if !bytes.HasPrefix(document, []byte("%PDF-1.")) {
		t.Fatalf("документ не является PDF: %q", document[:min(16, len(document))])
	}
	if len(document) < 10000 {
		t.Fatalf("PDF слишком мал: %d байт", len(document))
	}
	if !bytes.Contains(document, []byte("pdfaid:part")) {
		t.Fatal("в PDF нет XMP-метаданных PDF/A")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestPaperQRSheet(t *testing.T) {
	items := make([]PaperQRData, 9)
	for index := range items {
		items[index] = PaperQRData{
			EmployeeName:   "Иванов Иван Иванович",
			EmployeeNumber: "E-0001",
			Position:       "Инженер",
			Department:     "Отдел разработки",
			CompanyName:    "ООО «Ромашка»",
			Token:          "11111111-1111-1111-1111-111111111111",
			ValidFrom:      "2026-10-19",
			ValidUntil:     "2026-11-19",
			Status:         "active",
			QRContent:      `{"token":"11111111-1111-1111-1111-111111111111","signature":"test","payload":{"full_name":"Иванов Иван Иванович"}}`,
			SupportPhone:   "+7 999 000-00-00",
		}
	}
	document, err := PaperQRSheet(items)
	if err != nil {
		t.Fatalf("PaperQRSheet: %v", err)
	}
	if !bytes.HasPrefix(document, []byte("%PDF-1.")) {
		t.Fatalf("документ не является PDF: %q", document[:min(16, len(document))])
	}
	if pages := bytes.Count(document, []byte("/Type /Page\n")); pages != 2 {
		t.Fatalf("ожидалось 2 страницы A4, получено %d", pages)
	}
}
