// Package storage defines the storage interface for ShipItAI.
package storage

import (
	"context"
)

// Storage defines the interface for ShipItAI storage backends.
// Implementations must be safe for concurrent use by multiple goroutines.
type Storage interface {
	// Review operations
	StoreReview(ctx context.Context, review *ReviewContext) error
	GetReview(ctx context.Context, installationID int64, owner, repo string, prNumber int, reviewID int64) (*ReviewContext, error)
	ListReviewsForPR(ctx context.Context, installationID int64, owner, repo string, prNumber int) ([]*ReviewContext, error)
	GetFirstReviewForPR(ctx context.Context, installationID int64, owner, repo string, prNumber int) (*ReviewContext, error)

	// Installation operations
	SaveInstallation(ctx context.Context, install *Installation) error
	GetInstallation(ctx context.Context, installationID int64) (*Installation, error)
}
