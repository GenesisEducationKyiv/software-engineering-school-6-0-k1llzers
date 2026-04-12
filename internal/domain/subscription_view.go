package domain

type SubscriptionView struct {
	Email       string
	Repo        string
	Confirmed   bool
	LastSeenTag string
}
