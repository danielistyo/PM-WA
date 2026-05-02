package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

type Database struct {
	db *sql.DB
}

func New(dbPath string) (*Database, error) {
	connStr := "file:" + dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON"
	sqlDB, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)

	d := &Database{db: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *Database) Close() error {
	return d.db.Close()
}

func (d *Database) DB() *sql.DB {
	return d.db
}
