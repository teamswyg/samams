package notification

// Command carries data for the notification.created event handler.
type Command struct {
	UserID   string `json:"userId"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
}
