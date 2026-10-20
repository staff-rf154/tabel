package httpapi

import (
	"archive/zip"
	"bytes"
	"testing"
)

func zipBytes(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestReadBulkRecordsFormats(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		data     []byte
		want     int
		errors   int
	}{
		{
			name:     "массив",
			filename: "bulk.json",
			data:     []byte(`[{"employee_id":"1","payload":{}},{"employee_id":"2","payload":{}}]`),
			want:     2,
		},
		{
			name:     "объект с items",
			filename: "bulk.json",
			data:     []byte(`{"items":[{"employee_id":"1","payload":{}}]}`),
			want:     1,
		},
		{
			name:     "одиночный объект",
			filename: "bulk.json",
			data:     []byte(`{"employee_id":"1","payload":{}}`),
			want:     1,
		},
		{
			name:     "ndjson",
			filename: "bulk.ndjson",
			data:     []byte("{\"employee_id\":\"1\",\"payload\":{}}\n\n{\"employee_id\":\"2\",\"payload\":{}}\n"),
			want:     2,
		},
		{
			name:     "ndjson с ошибкой",
			filename: "bulk.ndjson",
			data:     []byte("{\"employee_id\":\"1\",\"payload\":{}}\n{broken}\n"),
			want:     2,
			errors:   1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			records, err := readBulkRecords(tc.filename, tc.data)
			if err != nil {
				t.Fatalf("readBulkRecords: %v", err)
			}
			if len(records) != tc.want {
				t.Fatalf("получено %d записей, ожидалось %d", len(records), tc.want)
			}
			failed := 0
			for _, record := range records {
				if record.ParseErr != nil {
					failed++
				}
			}
			if failed != tc.errors {
				t.Fatalf("ошибок разбора %d, ожидалось %d", failed, tc.errors)
			}
		})
	}
}

func TestReadBulkRecordsZip(t *testing.T) {
	data := zipBytes(t, "employees.json", []byte(`[{"employee_id":"1","payload":{}}]`))
	records, err := readBulkRecords("bulk.zip", data)
	if err != nil {
		t.Fatalf("readBulkRecords: %v", err)
	}
	if len(records) != 1 || records[0].File != "employees.json" {
		t.Fatalf("неожиданные записи: %+v", records)
	}

	if _, err := readBulkRecords("empty.json", []byte("  ")); err == nil {
		t.Fatal("ожидалась ошибка для пустого файла")
	}
}
