package main

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type application struct {
	db *sql.DB
}

type uploadResult struct {
	TotalItems      int         `json:"total_items"`
	TotalCategories int         `json:"total_categories"`
	TotalPrice      json.Number `json:"total_price"`
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func openDB() (*sql.DB, error) {
	host := env("POSTGRES_HOST", "127.0.0.1")
	if host == "localhost" {
		host = "127.0.0.1"
	}
	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(env("POSTGRES_USER", "validator"), env("POSTGRES_PASSWORD", "val1dat0r")),
		Host:   net.JoinHostPort(host, env("POSTGRES_PORT", "5432")),
		Path:   "/" + env("POSTGRES_DB", "project-sem-1"),
	}
	options := dsn.Query()
	options.Set("sslmode", env("POSTGRES_SSLMODE", "disable"))
	dsn.RawQuery = options.Encode()
	db, err := sql.Open("postgres", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	for attempt := 0; attempt < 30; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS prices (
		row_id BIGSERIAL PRIMARY KEY,
		id BIGINT NOT NULL,
		name TEXT NOT NULL,
		category TEXT NOT NULL,
		price NUMERIC(14, 2) NOT NULL,
		create_date DATE NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create prices table: %w", err)
	}
	return db, nil
}

func (app *application) upload(w http.ResponseWriter, r *http.Request) {
	archiveType := strings.ToLower(r.URL.Query().Get("type"))
	if archiveType == "" {
		archiveType = "zip"
	}
	if archiveType != "zip" && archiveType != "tar" {
		http.Error(w, "type must be zip or tar", http.StatusBadRequest)
		return
	}
	data, err := requestArchive(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	csvData, err := archiveCSV(data, archiveType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	items, err := parsePrices(csvData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := app.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, "database unavailable", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(r.Context(), `INSERT INTO prices (id, name, category, price, create_date) VALUES ($1, $2, $3, $4, $5)`)
	if err != nil {
		http.Error(w, "database unavailable", http.StatusInternalServerError)
		return
	}
	for _, item := range items {
		_, err = statement.ExecContext(r.Context(), item.ID, item.Name, item.Category, item.Price, item.CreateDate)
		if err != nil {
			statement.Close()
			http.Error(w, "could not save prices", http.StatusInternalServerError)
			return
		}
	}
	statement.Close()
	var categories int
	var totalPrice string
	if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(DISTINCT category), COALESCE(SUM(price), 0)::text FROM prices`).Scan(&categories, &totalPrice); err != nil {
		http.Error(w, "could not read price totals", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "could not save prices", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(uploadResult{len(items), categories, json.Number(totalPrice)})
}

func (app *application) download(w http.ResponseWriter, r *http.Request) {
	rows, err := app.db.QueryContext(r.Context(), `SELECT id, name, category, price::text, create_date::text FROM prices ORDER BY row_id`)
	if err != nil {
		http.Error(w, "database unavailable", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	file, err := archive.Create("data.csv")
	if err != nil {
		http.Error(w, "could not create archive", http.StatusInternalServerError)
		return
	}
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"id", "name", "category", "price", "create_date"}); err != nil {
		http.Error(w, "could not write CSV", http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var id int64
		var name, category, amount, created string
		if err := rows.Scan(&id, &name, &category, &amount, &created); err != nil {
			http.Error(w, "could not read prices", http.StatusInternalServerError)
			return
		}
		if err := writer.Write([]string{strconv.FormatInt(id, 10), name, category, amount, created}); err != nil {
			http.Error(w, "could not write CSV", http.StatusInternalServerError)
			return
		}
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "could not read prices", http.StatusInternalServerError)
		return
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		http.Error(w, "could not write CSV", http.StatusInternalServerError)
		return
	}
	if err := archive.Close(); err != nil {
		http.Error(w, "could not close archive", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="prices.zip"`)
	w.Write(output.Bytes())
}

func main() {
	db, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if len(os.Args) > 1 && os.Args[1] == "init-db" {
		return
	}
	app := &application{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v0/prices", app.upload)
	mux.HandleFunc("GET /api/v0/prices", app.download)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	server := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
