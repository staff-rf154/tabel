// Package pdf формирует печатные формы QR-токенов.
package pdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed fonts/DejaVuSans.ttf
var regularFont []byte

//go:embed fonts/DejaVuSans-Bold.ttf
var boldFont []byte

const (
	passWidthMM  = 85.6
	passHeightMM = 53.98

	sheetGapMM   = 4.0
	sheetLeftMM  = 5.0
	sheetTopMM   = 5.0
	sheetCols    = 2
	sheetRows    = 4
	sheetPerPage = sheetCols * sheetRows
)

type PaperQRData struct {
	EmployeeName   string
	EmployeeNumber string
	Position       string
	Department     string
	CompanyName    string
	Token          string
	ValidFrom      string
	ValidUntil     string
	Status         string
	QRContent      string
	SupportPhone   string
	SupportEmail   string
	PassFooter     string
}

// PaperQR генерирует пропуск 85,6 × 53,98 мм на отдельной странице.
func PaperQR(data PaperQRData) ([]byte, error) {
	doc := newPaperDoc("Бумажный пропуск", passWidthMM, passHeightMM)
	doc.AddPageFormat("P", fpdf.SizeType{Wd: passWidthMM, Ht: passHeightMM})
	if err := drawPaperPass(doc, data, 0, 0, "qr"); err != nil {
		return nil, err
	}
	return outputPaperDoc(doc)
}

// PaperQRSheet размещает пропуски на листах A4 по 8 штук (2 × 4).
func PaperQRSheet(items []PaperQRData) ([]byte, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("нет пропусков для печати")
	}
	doc := newPaperDoc("Бумажные пропуски", 210, 297)
	doc.SetMargins(0, 0, 0)
	doc.SetAutoPageBreak(false, 0)
	for pageStart := 0; pageStart < len(items); pageStart += sheetPerPage {
		doc.AddPageFormat("P", fpdf.SizeType{Wd: 210, Ht: 297})
		for index := pageStart; index < len(items) && index < pageStart+sheetPerPage; index++ {
			position := index - pageStart
			col := position % sheetCols
			row := position / sheetCols
			x := sheetLeftMM + float64(col)*(passWidthMM+sheetGapMM)
			y := sheetTopMM + float64(row)*(passHeightMM+sheetGapMM)
			name := fmt.Sprintf("qr%d", index)
			if err := drawPaperPass(doc, items[index], x, y, name); err != nil {
				return nil, err
			}
			doc.SetDrawColor(190, 195, 202)
			doc.SetLineWidth(0.15)
			doc.Line(x-2, y-1, x-2, y+passHeightMM+1)
			doc.Line(x-1, y-2, x+passWidthMM+1, y-2)
		}
	}
	return outputPaperDoc(doc)
}

func newPaperDoc(title string, width, height float64) *fpdf.Fpdf {
	doc := fpdf.NewCustom(&fpdf.InitType{
		OrientationStr: "P",
		UnitStr:        "mm",
		Size:           fpdf.SizeType{Wd: width, Ht: height},
	})
	doc.SetMargins(0, 0, 0)
	doc.SetAutoPageBreak(false, 0)
	doc.AddUTF8FontFromBytes("DejaVu", "", regularFont)
	doc.AddUTF8FontFromBytes("DejaVu", "B", boldFont)
	doc.SetTitle(title, true)
	doc.SetAuthor("Табель", true)
	doc.SetSubject("Пропуск для учёта рабочего времени", true)
	doc.SetCreator("Табель", true)
	doc.SetProducer("Табель", true)
	doc.SetKeywords("QR, табель, пропуск, учёт рабочего времени", true)
	doc.SetLang("ru-RU")
	doc.SetXmpMetadata(xmpMetadata(time.Now().UTC(), title))
	return doc
}

func outputPaperDoc(doc *fpdf.Fpdf) ([]byte, error) {
	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		return nil, fmt.Errorf("формирование PDF: %w", err)
	}
	return out.Bytes(), nil
}

func drawPaperPass(doc *fpdf.Fpdf, data PaperQRData, x, y float64, imageName string) error {
	const cardWidth = passWidthMM
	const cardHeight = passHeightMM
	company := strings.TrimSpace(data.CompanyName)
	if company == "" {
		company = "Табель"
	}

	doc.SetFillColor(255, 255, 255)
	doc.SetDrawColor(205, 210, 218)
	doc.SetLineWidth(0.35)
	doc.Rect(x, y, cardWidth, cardHeight, "DF")
	doc.SetFillColor(21, 83, 156)
	doc.Rect(x, y, cardWidth, 2.4, "F")

	doc.SetTextColor(21, 83, 156)
	doc.SetFont("DejaVu", "B", 6.5)
	doc.SetXY(x+3, y+3)
	doc.CellFormat(51, 4, fitText(doc, company, 51, 6.5, 4.2), "", 0, "L", false, 0, "")
	doc.SetTextColor(120, 128, 140)
	doc.SetFont("DejaVu", "B", 4.6)
	doc.SetXY(x+56, y+3.4)
	doc.CellFormat(26, 3, "ПРОПУСК", "", 0, "R", false, 0, "")

	photoX := x + 3
	photoY := y + 11
	photoW := 13.0
	photoH := 16.0
	doc.SetDrawColor(170, 177, 188)
	doc.SetLineWidth(0.25)
	doc.Rect(photoX, photoY, photoW, photoH, "")
	doc.SetFillColor(238, 241, 245)
	doc.Rect(photoX, photoY, photoW, photoH, "F")
	doc.SetDrawColor(170, 177, 188)
	doc.Rect(photoX, photoY, photoW, photoH, "")
	doc.Line(photoX, photoY, photoX+photoW, photoY+photoH)
	doc.Line(photoX+photoW, photoY, photoX, photoY+photoH)
	doc.SetTextColor(145, 152, 162)
	doc.SetFont("DejaVu", "B", 4.5)
	doc.SetXY(photoX, photoY+6.3)
	doc.CellFormat(photoW, 3, "ФОТО", "", 0, "C", false, 0, "")

	textX := x + 18.5
	textW := 37.0
	doc.SetTextColor(24, 30, 38)
	doc.SetFont("DejaVu", "B", 7.4)
	doc.SetXY(textX, y+11)
	doc.MultiCell(textW, 3.5, data.EmployeeName, "", "L", false)
	if data.Position != "" {
		doc.SetTextColor(55, 65, 78)
		doc.SetFont("DejaVu", "", 5.7)
		doc.SetXY(textX, y+19.5)
		doc.MultiCell(textW, 3.0, data.Position, "", "L", false)
	}
	if data.Department != "" {
		doc.SetTextColor(88, 98, 110)
		doc.SetFont("DejaVu", "", 5.2)
		doc.SetXY(textX, y+27.5)
		doc.MultiCell(textW, 2.8, data.Department, "", "L", false)
	}

	png, err := qrcode.Encode(data.QRContent, qrcode.Medium, 768)
	if err != nil {
		return fmt.Errorf("генерация QR: %w", err)
	}
	doc.RegisterImageOptionsReader(imageName, fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(png))
	qrX := x + cardWidth - 27.5
	qrY := y + 11
	doc.ImageOptions(imageName, qrX, qrY, 24, 24, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	doc.SetDrawColor(220, 224, 230)
	doc.SetLineWidth(0.2)
	doc.Rect(qrX-0.8, qrY-0.8, 25.6, 25.6, "")

	doc.SetTextColor(45, 54, 66)
	doc.SetFont("DejaVu", "", 4.8)
	doc.SetXY(x+3, y+35.4)
	doc.CellFormat(48, 2.8, "Табельный номер: "+data.EmployeeNumber, "", 0, "L", false, 0, "")
	doc.SetXY(x+3, y+39.0)
	doc.CellFormat(70, 2.8, "Действителен: "+data.ValidFrom+" — "+data.ValidUntil, "", 0, "L", false, 0, "")

	if data.Status != "" && data.Status != "active" {
		label := "ТОКЕН ОТОЗВАН"
		if data.Status == "expired" {
			label = "СРОК ДЕЙСТВИЯ ИСТЁК"
		}
		doc.SetFillColor(190, 32, 38)
		doc.SetTextColor(255, 255, 255)
		doc.SetFont("DejaVu", "B", 4.8)
		doc.SetXY(x+3, y+42.1)
		doc.CellFormat(42, 3.4, label, "", 0, "C", true, 0, "")
	}

	doc.SetDrawColor(220, 224, 230)
	doc.Line(x+3, y+46.0, x+cardWidth-3, y+46.0)
	doc.SetTextColor(110, 118, 130)
	doc.SetFont("DejaVu", "", 4.0)
	footer := strings.TrimSpace(data.PassFooter)
	if footer == "" {
		footer = "Система «Табель» · учёт рабочего времени"
	}
	doc.SetXY(x+3, y+46.7)
	doc.CellFormat(45, 2.4, fitText(doc, footer, 45, 4.0, 3.2), "", 0, "L", false, 0, "")
	contact := strings.TrimSpace(data.SupportPhone)
	if contact == "" {
		contact = strings.TrimSpace(data.SupportEmail)
	}
	if contact != "" {
		doc.SetXY(x+cardWidth-39, y+46.7)
		doc.CellFormat(36, 2.4, fitText(doc, "Поддержка: "+contact, 36, 4.0, 3.2), "", 0, "R", false, 0, "")
	}
	doc.SetTextColor(150, 156, 166)
	doc.SetFont("DejaVu", "", 3.6)
	doc.SetXY(x+3, y+49.6)
	doc.CellFormat(cardWidth-6, 2.2, "ID: "+shortToken(data.Token), "", 0, "L", false, 0, "")
	return nil
}

func fitText(doc *fpdf.Fpdf, text string, maxWidth, size, minSize float64) string {
	for size >= minSize {
		doc.SetFontSize(size)
		if doc.GetStringWidth(text) <= maxWidth {
			return text
		}
		size -= 0.2
	}
	return text
}

func shortToken(token string) string {
	if len(token) <= 16 {
		return token
	}
	return token[:8] + "…" + token[len(token)-6:]
}

func xmpMetadata(created time.Time, title string) []byte {
	return []byte(`<?xpacket begin="` + "\ufeff" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/">
   <pdfaid:part>1</pdfaid:part>
   <pdfaid:conformance>B</pdfaid:conformance>
  </rdf:Description>
  <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">
   <dc:title><rdf:Alt><rdf:li xml:lang="x-default">` + html.EscapeString(title) + `</rdf:li></rdf:Alt></dc:title>
   <dc:creator><rdf:Seq><rdf:li>Табель</rdf:li></rdf:Seq></dc:creator>
  </rdf:Description>
  <rdf:Description rdf:about="" xmlns:xmp="http://ns.adobe.com/xap/1.0/">
   <xmp:CreatorTool>Табель</xmp:CreatorTool>
   <xmp:CreateDate>` + created.Format("2006-01-02T15:04:05Z") + `</xmp:CreateDate>
  </rdf:Description>
  <rdf:Description rdf:about="" xmlns:pdf="http://ns.adobe.com/pdf/1.3/">
   <pdf:Producer>Табель</pdf:Producer>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`)
}
