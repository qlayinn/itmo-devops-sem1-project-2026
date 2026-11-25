package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	maxArchiveSize = 64 << 20
	maxCSVSize     = 128 << 20
)

type price struct {
	ID         int64
	Name       string
	Category   string
	Price      string
	CreateDate time.Time
}

func requestArchive(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxArchiveSize+1)
	var input io.Reader = r.Body
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		file, _, err := r.FormFile("file")
		if err != nil {
			return nil, err
		}
		defer file.Close()
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		input = file
	}
	contents, err := io.ReadAll(io.LimitReader(input, maxArchiveSize+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxArchiveSize {
		return nil, errors.New("archive is too large")
	}
	return contents, nil
}

func readLimited(reader io.Reader) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(reader, maxCSVSize+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxCSVSize {
		return nil, errors.New("data.csv is too large")
	}
	return contents, nil
}

func archiveCSV(data []byte, archiveType string) ([]byte, error) {
	isDataFile := func(name string) bool {
		base := path.Base(name)
		return base == "data.csv" || base == "test_data.csv"
	}
	switch archiveType {
	case "zip":
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if !isDataFile(file.Name) || file.FileInfo().IsDir() {
				continue
			}
			stream, err := file.Open()
			if err != nil {
				return nil, err
			}
			contents, err := readLimited(stream)
			stream.Close()
			return contents, err
		}
	case "tar":
		reader := tar.NewReader(bytes.NewReader(data))
		for {
			header, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if header.Typeflag == tar.TypeReg && isDataFile(header.Name) {
				return readLimited(reader)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported archive type %q", archiveType)
	}
	return nil, errors.New("archive has no data.csv or test_data.csv")
}

func parsePrices(data []byte) ([]price, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[strings.ToLower(strings.TrimSpace(name))] = index
	}
	for _, name := range []string{"id", "name", "category", "price", "create_date"} {
		if _, ok := columns[name]; !ok {
			return nil, fmt.Errorf("missing CSV column %q", name)
		}
	}
	var result []price
	for line := 2; ; line++ {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: %w", line, err)
		}
		if len(row) != len(header) {
			return nil, fmt.Errorf("CSV line %d: expected %d columns, got %d", line, len(header), len(row))
		}
		id, err := strconv.ParseInt(strings.TrimSpace(row[columns["id"]]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: invalid id: %w", line, err)
		}
		amount := strings.TrimSpace(row[columns["price"]])
		if _, err := strconv.ParseFloat(amount, 64); err != nil {
			return nil, fmt.Errorf("CSV line %d: invalid price: %w", line, err)
		}
		created, err := time.Parse("2006-01-02", strings.TrimSpace(row[columns["create_date"]]))
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: invalid date: %w", line, err)
		}
		result = append(result, price{
			ID: id, Name: row[columns["name"]], Category: row[columns["category"]],
			Price: amount, CreateDate: created,
		})
	}
	return result, nil
}
