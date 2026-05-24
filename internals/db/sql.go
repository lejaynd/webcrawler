package db

import (
	"database/sql"
	"fmt"
	"time"
)

type SQLStore struct {
	DB *sql.DB
}

func NewSQLStore(dsn string) (*SQLStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &SQLStore{DB: db}, nil
}

func (s *SQLStore) CreateTable() error {
	_, err := s.DB.Exec(`
		CREATE TABLE IF NOT EXISTS crawl_records (
			url        TEXT PRIMARY KEY,
			s3_link    TEXT NOT NULL,
			depth      INT  NOT NULL,
			visited_at TIMESTAMPTZ NOT NULL
		)
	`)
	return err
}

func (s *SQLStore) InsertRecord(pageURL, s3Link string, depth int) error {
	_, err := s.DB.Exec(`
		INSERT INTO crawl_records (url, s3_link, depth, visited_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (url) DO UPDATE
			SET s3_link    = EXCLUDED.s3_link,
			    visited_at = EXCLUDED.visited_at
	`, pageURL, s3Link, depth, time.Now().UTC())
	return err
}
