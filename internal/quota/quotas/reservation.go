package quotas

type ReservationStatus string

const (
	ReservationStatusReserved  ReservationStatus = "reserved"
	ReservationStatusCommitted ReservationStatus = "committed"
	ReservationStatusReleased  ReservationStatus = "released"
	ReservationStatusRejected  ReservationStatus = "rejected"
)

type ReserveResult struct {
	Reserved        bool
	RejectionReason string
}
