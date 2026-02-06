// Package postgres provides a PostgreSQL implementation of the storage interface.
// This is intended for self-hosted deployments.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shipitai/shipitai/storage"
)

// PostgreSQL provides storage operations using PostgreSQL.
type PostgreSQL struct {
	db *sql.DB
}

// New creates a new PostgreSQL storage instance.
func New(db *sql.DB) *PostgreSQL {
	return &PostgreSQL{db: db}
}

// NewFromDSN creates a new PostgreSQL storage instance from a connection string.
func NewFromDSN(dsn string) (*PostgreSQL, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &PostgreSQL{db: db}, nil
}

// Close closes the database connection.
func (p *PostgreSQL) Close() error {
	return p.db.Close()
}

// Migrate creates the required database tables.
func (p *PostgreSQL) Migrate(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS installations (
			installation_id BIGINT PRIMARY KEY,
			account_id BIGINT,
			org_login TEXT NOT NULL,
			installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			installed_by TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS reviews (
			id SERIAL PRIMARY KEY,
			installation_id BIGINT NOT NULL,
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			review_id BIGINT NOT NULL,
			review_body TEXT,
			comments JSONB,
			usage JSONB,
			usage_type TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(installation_id, owner, repo, pr_number, review_id)
		);

		CREATE INDEX IF NOT EXISTS idx_reviews_pr ON reviews(installation_id, owner, repo, pr_number);
	`

	_, err := p.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// StoreReview stores a review context in PostgreSQL.
func (p *PostgreSQL) StoreReview(ctx context.Context, review *storage.ReviewContext) error {
	query := `
		INSERT INTO reviews (installation_id, owner, repo, pr_number, review_id, review_body, comments, usage, usage_type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (installation_id, owner, repo, pr_number, review_id) DO UPDATE SET
			review_body = EXCLUDED.review_body,
			comments = EXCLUDED.comments,
			usage = EXCLUDED.usage,
			usage_type = EXCLUDED.usage_type
	`

	_, err := p.db.ExecContext(ctx, query,
		review.InstallationID,
		review.Owner,
		review.Repo,
		review.PRNumber,
		review.ReviewID,
		review.ReviewBody,
		commentsToJSON(review.Comments),
		usageToJSON(review.Usage),
		review.UsageType,
	)
	if err != nil {
		return fmt.Errorf("failed to store review: %w", err)
	}

	return nil
}

// GetReview retrieves a review context from PostgreSQL.
func (p *PostgreSQL) GetReview(ctx context.Context, installationID int64, owner, repo string, prNumber int, reviewID int64) (*storage.ReviewContext, error) {
	query := `
		SELECT installation_id, owner, repo, pr_number, review_id, review_body, comments, usage, usage_type, created_at
		FROM reviews
		WHERE installation_id = $1 AND owner = $2 AND repo = $3 AND pr_number = $4 AND review_id = $5
	`

	var review storage.ReviewContext
	var commentsJSON, usageJSON sql.NullString
	var createdAt time.Time

	err := p.db.QueryRowContext(ctx, query, installationID, owner, repo, prNumber, reviewID).Scan(
		&review.InstallationID,
		&review.Owner,
		&review.Repo,
		&review.PRNumber,
		&review.ReviewID,
		&review.ReviewBody,
		&commentsJSON,
		&usageJSON,
		&review.UsageType,
		&createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get review: %w", err)
	}

	review.Comments = commentsFromJSON(commentsJSON.String)
	review.Usage = usageFromJSON(usageJSON.String)
	review.CreatedAt = createdAt.Format(time.RFC3339)

	return &review, nil
}

// ListReviewsForPR retrieves all reviews for a pull request.
func (p *PostgreSQL) ListReviewsForPR(ctx context.Context, installationID int64, owner, repo string, prNumber int) ([]*storage.ReviewContext, error) {
	query := `
		SELECT installation_id, owner, repo, pr_number, review_id, review_body, comments, usage, usage_type, created_at
		FROM reviews
		WHERE installation_id = $1 AND owner = $2 AND repo = $3 AND pr_number = $4
		ORDER BY created_at ASC
	`

	rows, err := p.db.QueryContext(ctx, query, installationID, owner, repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to list reviews: %w", err)
	}
	defer rows.Close()

	var reviews []*storage.ReviewContext
	for rows.Next() {
		var review storage.ReviewContext
		var commentsJSON, usageJSON sql.NullString
		var createdAt time.Time

		if err := rows.Scan(
			&review.InstallationID,
			&review.Owner,
			&review.Repo,
			&review.PRNumber,
			&review.ReviewID,
			&review.ReviewBody,
			&commentsJSON,
			&usageJSON,
			&review.UsageType,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan review: %w", err)
		}

		review.Comments = commentsFromJSON(commentsJSON.String)
		review.Usage = usageFromJSON(usageJSON.String)
		review.CreatedAt = createdAt.Format(time.RFC3339)
		reviews = append(reviews, &review)
	}

	return reviews, rows.Err()
}

// GetFirstReviewForPR retrieves the first (oldest) review for a pull request.
func (p *PostgreSQL) GetFirstReviewForPR(ctx context.Context, installationID int64, owner, repo string, prNumber int) (*storage.ReviewContext, error) {
	query := `
		SELECT installation_id, owner, repo, pr_number, review_id, review_body, comments, usage, usage_type, created_at
		FROM reviews
		WHERE installation_id = $1 AND owner = $2 AND repo = $3 AND pr_number = $4
		ORDER BY created_at ASC
		LIMIT 1
	`

	var review storage.ReviewContext
	var commentsJSON, usageJSON sql.NullString
	var createdAt time.Time

	err := p.db.QueryRowContext(ctx, query, installationID, owner, repo, prNumber).Scan(
		&review.InstallationID,
		&review.Owner,
		&review.Repo,
		&review.PRNumber,
		&review.ReviewID,
		&review.ReviewBody,
		&commentsJSON,
		&usageJSON,
		&review.UsageType,
		&createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get first review: %w", err)
	}

	review.Comments = commentsFromJSON(commentsJSON.String)
	review.Usage = usageFromJSON(usageJSON.String)
	review.CreatedAt = createdAt.Format(time.RFC3339)

	return &review, nil
}

// SaveInstallation stores a new installation.
func (p *PostgreSQL) SaveInstallation(ctx context.Context, install *storage.Installation) error {
	query := `
		INSERT INTO installations (installation_id, account_id, org_login, installed_by, installed_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (installation_id) DO UPDATE SET
			org_login = EXCLUDED.org_login,
			updated_at = NOW()
	`

	installedAt := time.Now()
	if install.InstalledAt != "" {
		if t, err := time.Parse(time.RFC3339, install.InstalledAt); err == nil {
			installedAt = t
		}
	}

	_, err := p.db.ExecContext(ctx, query,
		install.InstallationID,
		install.AccountID,
		install.OrgLogin,
		install.InstalledBy,
		installedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save installation: %w", err)
	}

	return nil
}

// GetInstallation retrieves an installation.
func (p *PostgreSQL) GetInstallation(ctx context.Context, installationID int64) (*storage.Installation, error) {
	query := `
		SELECT installation_id, account_id, org_login, installed_at, installed_by
		FROM installations
		WHERE installation_id = $1
	`

	var install storage.Installation
	var installedAt time.Time
	var accountID sql.NullInt64
	var installedBy sql.NullString

	err := p.db.QueryRowContext(ctx, query, installationID).Scan(
		&install.InstallationID,
		&accountID,
		&install.OrgLogin,
		&installedAt,
		&installedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get installation: %w", err)
	}

	install.AccountID = accountID.Int64
	install.InstalledBy = installedBy.String
	install.InstalledAt = installedAt.Format(time.RFC3339)

	return &install, nil
}

// Verify PostgreSQL implements Storage at compile time.
var _ storage.Storage = (*PostgreSQL)(nil)
