package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
)

type Config struct {
	Host string
}

func Parse() (Config, error) {
	if len(os.Args) < 2 {
		return Config{}, errors.New("not enough arguments")
	}

	return Config{
		Host: os.Args[1],
	}, nil
}

func main() {
	config, err := Parse()
	if err != nil {
		log.Fatalf("failed to parse command-line arguments: %s", err.Error())
	}

	db := make(map[string]string, 0)

	mux := http.NewServeMux()
	mux.HandleFunc("/shorten", shorten(config, db))
	mux.HandleFunc("/", get(db))

	fmt.Println("Starting Server at 0.0.0.0:8080")
	if err := http.ListenAndServe("0.0.0.0:8080", mux); err != nil {
		log.Printf("ListenAndServe: %s", err.Error())
	}
}

type H = map[string]string

type input struct {
	OriginalLink string `json:"originalLink"`
}

func shorten(config Config, db map[string]string) func(w http.ResponseWriter, r *http.Request) {
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

		fmt.Printf("shorten: %s", body.OriginalLink)

		hash, err := saveLink(db, body.OriginalLink)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		shortLink := fmt.Sprintf("%s/%s", config.Host, hash)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(H{"shortlink": shortLink})
	}
}

func get(db map[string]string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		hash := r.URL.Path[1:]

		if original, ok := db[hash]; ok {
			http.Redirect(w, r, original, http.StatusFound)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(H{"error": "original link not found"})
	}
}

func saveLink(db map[string]string, original string) (string, error) {
	id := rand.Int63()
	input := fmt.Sprintf("%s%d", original, id)
	hashFunc := sha256.New()
	hashFunc.Write([]byte(input))
	hash := base64.URLEncoding.EncodeToString(hashFunc.Sum(nil))[:10]

	db[hash] = original
	return hash, nil
}
