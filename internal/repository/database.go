package repository

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/kasbench/globeco-allocation-service/internal/config"
)

// DB wraps sqlx.DB with additional functionality
type DB struct {
	*sqlx.DB
	logger *zap.Logger
}

// NewPostgresDB creates a new PostgreSQL database connection
func NewPostgresDB(cfg config.Database) (*DB, error) {
	db, err := sqlx.Connect("postgres", cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// --- Debug: List /migrations directory ---
	files, err := ioutil.ReadDir("/migrations")
	if err != nil {
		log.Printf("[DEBUG] Could not read /migrations: %v", err)
	} else {
		log.Printf("[DEBUG] Listing /migrations:")
		for _, f := range files {
			log.Printf("[DEBUG]   %s", f.Name())
		}
	}
	// --- End debug ---

	// --- Automatic migration ---
	migrationsPath := "/migrations"
	driver, err := postgres.WithInstance(db.DB, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create migration driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"postgres", driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize migrate: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return nil, fmt.Errorf("database migration failed: %w", err)
	}
	// --- End migration ---

	return &DB{
		DB:     db,
		logger: zap.NewNop(), // Will be replaced by caller
	}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// SetLogger sets the logger for the database
func (db *DB) SetLogger(logger *zap.Logger) {
	db.logger = logger
}

// HealthCheck performs a health check on the database
func (db *DB) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result int
	err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("unexpected health check result: %d", result)
	}

	return nil
}
