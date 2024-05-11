package queue

import (
	faktory "github.com/contribsys/faktory/client"
	"time"
)

type Helper interface {
	Jid() string
	JobType() string

	// Custom provides access to the job custom hash.
	// Returns the value and `ok=true` if the key was found.
	// If not, returns `nil` and `ok=false`.
	//
	// No type checking is performed, please use with caution.
	Custom(key string) (value interface{}, ok bool)

	// Faktory Enterprise:
	// the BID of the Batch associated with this job
	Bid() string

	// Faktory Enterprise:
	// the BID of the Batch associated with this callback (complete or success) job
	CallbackBid() string

	// Faktory Enterprise:
	// open the batch associated with this job so we can add more jobs to it.
	//
	//   func myJob(ctx context.Context, args ...interface{}) error {
	//     helper := worker.HelperFor(ctx)
	//     helper.Batch(func(b *faktory.Batch) error {
	//       return b.Push(faktory.NewJob("sometype", 1, 2, 3))
	//     })
	Batch(func(*faktory.Batch) error) error

	// allows direct access to the Faktory server from the job
	With(func(*faktory.Client) error) error

	// Faktory Enterprise:
	// this method integrates with Faktory Enterprise's Job Tracking feature.
	// `reserveUntil` is optional, only needed for long jobs which have more dynamic
	// lifetimes.
	//
	//     helper.TrackProgress(10, "Updating code...", nil)
	//     helper.TrackProgress(20, "Cleaning caches...", &time.Now().Add(1 * time.Hour)))
	//
	TrackProgress(percent int, desc string, reserveUntil *time.Time) error
}
