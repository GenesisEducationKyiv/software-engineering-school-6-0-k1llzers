package domain

import "time"

type Release struct {
	TagName     string
	Name        string
	HTMLURL     string
	Draft       bool
	Prerelease  bool
	PublishedAt time.Time
}
