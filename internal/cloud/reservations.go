package cloud

import (
	"context"

	computev1 "google.golang.org/api/compute/v1"
)

// ReservationProvider holds the client for the GCP Compute Engine API
// and is used to interact with reservations.
type ReservationProvider struct {
	Service   *computev1.Service
	ProjectID string
	Zone      string
}

// GetReservation gets a reservation by name.
func (r *ReservationProvider) GetReservation(ctx context.Context, name string) (*computev1.Reservation, error) {
	return r.Service.Reservations.Get(r.ProjectID, r.Zone, name).Context(ctx).Do()
}
