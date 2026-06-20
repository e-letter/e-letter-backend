package repository

type NotificationPublisher interface {
	Publish(userID int, event string)
}
