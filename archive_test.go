package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func testArchive(t *testing.T, kind, name, contents string) []byte {
	t.Helper()
	var output bytes.Buffer
	switch kind {
	case "zip":
		writer := zip.NewWriter(&output)
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	case "tar":
		writer := tar.NewWriter(&output)
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return output.Bytes()
}

func TestArchiveCSV(t *testing.T) {
	const data = "id,name,category,price,create_date\n1,Tea,Food,42.50,2024-01-01\n"
	for _, kind := range []string{"zip", "tar"} {
		t.Run(kind, func(t *testing.T) {
			csvData, err := archiveCSV(testArchive(t, kind, "data.csv", data), kind)
			if err != nil {
				t.Fatal(err)
			}
			if string(csvData) != data {
				t.Fatalf("got %q, want %q", csvData, data)
			}
		})
	}
}

func TestTemplateArchiveName(t *testing.T) {
	const data = "id,name,category,price,create_date\n1,Tea,Food,42.50,2024-01-01\n"
	csvData, err := archiveCSV(testArchive(t, "zip", "test_data.csv", data), "zip")
	if err != nil || string(csvData) != data {
		t.Fatalf("got %q, error %v", csvData, err)
	}
}

func TestParsePricesByColumnName(t *testing.T) {
	data := []byte("id,create_date,name,category,price\n7,2024-03-02,Tea,Food,42.50\n")
	items, err := parsePrices(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != 7 || items[0].Price != "42.50" || items[0].CreateDate.Format("2006-01-02") != "2024-03-02" {
		t.Fatalf("unexpected result: %+v", items)
	}
}

func TestParsePricesMissingColumn(t *testing.T) {
	_, err := parsePrices([]byte("id,name,category,price\n1,Tea,Food,42.50\n"))
	if err == nil || !strings.Contains(err.Error(), "create_date") {
		t.Fatalf("expected missing column error, got %v", err)
	}
}
