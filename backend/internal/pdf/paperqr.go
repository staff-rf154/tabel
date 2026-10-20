// Package pdf формирует печатные формы QR-токенов.
package pdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"time"

	"github.com/go-pdf/fpdf"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed fonts/DejaVuSans.ttf
var regularFont []byte

//go:embed fonts/DejaVuSans-Bold.ttf
var boldFont []byte

type PaperQRData struct {
	EmployeeName   string
	EmployeeNumber string
	Position       string
	Token          string
	ValidFrom      string
	ValidUntil     string
	Status         string
	QRContent      string
	SupportPhone   string
}

// PaperQR генерирует одностраничную печатную форму бумажного QR-токена.
// Документ содержит XMP-метаданные PDF/A-1b и встроенные Unicode-шрифты.
func PaperQR(data PaperQRData) ([]byte, error) {
	doc := fpdf.New("P", "mm", "A4", "")
	doc.SetMargins(15, 15, 15)
	doc.SetAutoPageBreak(true, 15)
	doc.AddUTF8FontFromBytes("DejaVu", "", regularFont)
	doc.AddUTF8FontFromBytes("DejaVu", "B", boldFont)

	doc.SetTitle("Бумажный QR-токен сотрудника", true)
	doc.SetAuthor("Табель", true)
	doc.SetSubject("Бумажный QR-токен для учёта рабочего времени", true)
	doc.SetCreator("Табель", true)
	doc.SetProducer("Табель", true)
	doc.SetKeywords("QR, табель, учёт рабочего времени", true)
	doc.SetLang("ru-RU")
	doc.SetXmpMetadata(xmpMetadata(time.Now().UTC(), "Бумажный QR-токен сотрудника"))

	doc.AddPage()
	doc.SetDrawColor(210, 214, 220)
	doc.SetLineWidth(0.4)
	doc.Rect(12, 12, 186, 273, "")
	doc.SetDrawColor(30, 90, 160)
	doc.SetLineWidth(1.2)
	doc.Line(15, 34, 195, 34)

	doc.SetTextColor(30, 90, 160)
	doc.SetFont("DejaVu", "B", 24)
	doc.SetXY(15, 16)
	doc.CellFormat(0, 12, "Табель", "", 0, "L", false, 0, "")
	doc.SetTextColor(90, 100, 115)
	doc.SetFont("DejaVu", "", 11)
	doc.SetXY(15, 27)
	doc.CellFormat(0, 6, "учёт рабочего времени", "", 0, "L", false, 0, "")

	doc.SetTextColor(30, 30, 30)
	doc.SetFont("DejaVu", "B", 16)
	doc.SetXY(15, 42)
	doc.CellFormat(0, 10, "Бумажный QR-токен", "", 1, "L", false, 0, "")

	doc.SetFont("DejaVu", "", 11)
	doc.SetX(15)
	doc.CellFormat(0, 7, "Сотрудник: "+data.EmployeeName, "", 1, "L", false, 0, "")
	doc.SetX(15)
	doc.CellFormat(0, 7, "Табельный номер: "+data.EmployeeNumber, "", 1, "L", false, 0, "")
	if data.Position != "" {
		doc.SetX(15)
		doc.CellFormat(0, 7, "Должность: "+data.Position, "", 1, "L", false, 0, "")
	}

	png, err := qrcode.Encode(data.QRContent, qrcode.Medium, 768)
	if err != nil {
		return nil, fmt.Errorf("генерация QR: %w", err)
	}
	doc.RegisterImageOptionsReader("qr", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(png))
	doc.ImageOptions("qr", 15, 78, 78, 78, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	doc.SetDrawColor(210, 214, 220)
	doc.SetLineWidth(0.3)
	doc.Rect(14, 77, 80, 80, "")

	doc.SetXY(102, 78)
	doc.SetFont("DejaVu", "B", 12)
	doc.CellFormat(0, 7, "Порядок использования", "", 1, "L", false, 0, "")
	doc.SetFont("DejaVu", "", 10)
	doc.SetX(102)
	doc.MultiCell(92, 5.5,
		"1. Откройте мобильное приложение «Табель» и выберите отметку прихода или ухода.\n"+
			"2. Наведите камеру на QR-код.\n"+
			"3. Дождитесь подтверждения отметки.\n\n"+
			"Если код не считывается или срок действия истёк, обратитесь к администратору.",
		"", "L", false)

	doc.SetXY(15, 165)
	doc.SetFont("DejaVu", "B", 11)
	doc.CellFormat(0, 7, "Срок действия", "", 1, "L", false, 0, "")
	doc.SetFont("DejaVu", "", 11)
	doc.SetX(15)
	doc.CellFormat(0, 7, "с "+data.ValidFrom+" по "+data.ValidUntil+" включительно (Asia/Novosibirsk)", "", 1, "L", false, 0, "")

	if data.Status != "" && data.Status != "active" {
		label := "ТОКЕН ОТОЗВАН"
		if data.Status == "expired" {
			label = "СРОК ДЕЙСТВИЯ ИСТЁК"
		}
		doc.SetTextColor(180, 30, 30)
		doc.SetFont("DejaVu", "B", 13)
		doc.SetXY(15, 184)
		doc.CellFormat(0, 8, label, "", 1, "L", false, 0, "")
		doc.SetTextColor(30, 30, 30)
	}

	doc.SetDrawColor(210, 214, 220)
	doc.SetLineWidth(0.4)
	doc.Line(15, 248, 195, 248)
	doc.SetFont("DejaVu", "", 9)
	doc.SetTextColor(100, 105, 115)
	doc.SetXY(15, 252)
	doc.CellFormat(0, 5, "Идентификатор токена: "+data.Token, "", 1, "L", false, 0, "")
	if data.SupportPhone != "" {
		doc.SetX(15)
		doc.CellFormat(0, 5, "Техническая поддержка: "+data.SupportPhone, "", 1, "L", false, 0, "")
	}
	doc.SetX(15)
	doc.CellFormat(0, 5, "Документ сформирован автоматически системой «Табель».", "", 1, "L", false, 0, "")

	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		return nil, fmt.Errorf("формирование PDF: %w", err)
	}
	return out.Bytes(), nil
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
