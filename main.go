package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	_ "modernc.org/sqlite"
)

const (
	addr = "localhost:8080"
)

func main() {
	db, err := sql.Open("sqlite", "link.db")
	if err != nil {
		log.Fatalf("failed to open database: %s", err.Error())
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS links (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			original TEXT NOT NULL,
			hash TEXT
		)`)

	if err != nil {
		log.Fatalf("failed to create table [links]: %s", err.Error())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/shorten", shorten(db))
	mux.HandleFunc("/", get(db))
}

type H = map[string]string

type input struct {
	OriginalLink string `json:"originalLink"`
}

func shorten(db *sql.DB) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		d := json.NewDecoder(r.Body)
		body := input{}
		if err := d.Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		hash, err := saveLink(db, body.OriginalLink)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		shortLink := fmt.Sprintf("%s/%s", addr, hash)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(H{"shortlink": shortLink})
	}
}

func get(db *sql.DB) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		hash := r.URL.Path

		var original string
		row := db.QueryRow("SELECT original FROM links WHERE hash = ?", hash)
		err := row.Scan(&original)
		if err != nil {
			if err == sql.ErrNoRows {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(H{"error": "original link not found"})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, original, http.StatusFound)
	}
}

func saveLink(db *sql.DB, original string) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	result, err := tx.Exec("INSERT INTO links (original) values (?)", original)
	if err != nil {
		log.Printf("failed to insert original link: %s", err.Error())
		return "", err
	}

	id, err := result.LastInsertId()
	if err != nil {
		log.Printf("failed to get id: %s", err.Error())
		return "", err
	}

	input := fmt.Sprintf("%s%d", original, id)
	hashFunc := sha256.New()
	hashFunc.Write([]byte(input))
	hash := base64.URLEncoding.EncodeToString(hashFunc.Sum(nil))[:10]

	_, err = tx.Exec("UPDATE links SET hash = ? WHERE id = ?", hash, id)
	if err != nil {
		log.Printf("failed to set hash: %s", err.Error())
		return "", err
	}

	if err := tx.Commit(); err != nil {
		log.Printf("failed to commit transaction: %s", err.Error())
		return "", err
	}

	return hash, nil
}
