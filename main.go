package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"

	_ "modernc.org/sqlite"

	"github.com/gin-gonic/gin"
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

	http

	server := gin.Default()

	server.POST("/shorten", shorten(db))
	server.GET("/:hash", get(db))
	err = server.Run(addr)
	if err != nil {
		log.Fatalf("failed to run HTTP server: %s", err.Error())
	}
}

type input struct {
	OriginalLink string `json:"originalLink"`
}

func shorten(db *sql.DB) func(*gin.Context) {
	return func(c *gin.Context) {
		body := input{}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid input format"})
			return
		}

		tx, err := db.Begin()
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		result, err := tx.Exec("INSERT INTO links (original) values (?)", body.OriginalLink)
		if err != nil {
			log.Printf("failed to insert original link: %s", err.Error())
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		id, err := result.LastInsertId()
		if err != nil {
			log.Printf("failed to get id: %s", err.Error())
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		input := fmt.Sprintf("%s%d", body.OriginalLink, id)
		hashFunc := sha256.New()
		hashFunc.Write([]byte(input))
		hash := base64.URLEncoding.EncodeToString(hashFunc.Sum(nil))[:10]

		_, err = tx.Exec("UPDATE links SET hash = ? WHERE id = ?", hash, id)
		if err != nil {
			log.Printf("failed to set hash: %s", err.Error())
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("failed to commit transaction: %s", err.Error())
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		shortLink := fmt.Sprintf("%s/%s", addr, hash)
		c.JSON(http.StatusCreated, gin.H{"shortLink": shortLink})
	}
}

func get(db *sql.DB) func(*gin.Context) {
	return func(c *gin.Context) {
		hash, ok := c.Params.Get("hash")
		if !ok {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "hash not presented"})
			return
		}

		var original string
		row := db.QueryRow("SELECT original FROM links WHERE hash = ?", hash)
		err := row.Scan(&original)
		if err != nil {
			if err == sql.ErrNoRows {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "original link not found"})
				return
			}

			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		c.Redirect(http.StatusFound, original)

	}
}
