package domain

import "time"

// ActivityType is how Discord words the activity, as in "Playing". The codec
// maps it to Discord's number.
type ActivityType int

// The activity types.
const (
	ActivityPlaying ActivityType = iota
	ActivityListening
	ActivityWatching
	ActivityCompeting
)

// Activity is what Discord displays. The renderer produces it and the Discord
// codec encodes it. An empty field is left out of the payload.
type Activity struct {
	// Details and State are the first and second text lines.
	Details string
	State   string
	// Start is the beginning of the elapsed timer. Zero means no timer.
	Start time.Time
	// The image fields are asset keys; the text fields are their hover text.
	LargeImage string
	LargeText  string
	SmallImage string
	SmallText  string
	Type       ActivityType
}

// Equal reports whether Discord would display a and b the same, so that a
// duplicate can be dropped. Start is compared as an instant.
func (a Activity) Equal(b Activity) bool {
	sameStart := a.Start.Equal(b.Start)
	a.Start, b.Start = time.Time{}, time.Time{}
	return sameStart && a == b
}
