package service

import (
	"context"

	"github.com/Refliqx/backend-eletter/internal/domain"
	"github.com/Refliqx/backend-eletter/internal/repository"
)

type NotificationService interface {
	GetNotifications(ctx context.Context, userID int64) ([]domain.Notification, error)
	MarkAsRead(ctx context.Context, notificationID, userID int64) error
}

type notificationService struct {
	repo repository.NotificationRepository
}

func NewNotificationService(repo repository.NotificationRepository) NotificationService {
	return &notificationService{repo: repo}
}

func (s *notificationService) GetNotifications(ctx context.Context, userID int64) ([]domain.Notification, error) {
	return s.repo.GetByUser(ctx, userID)
}

func (s *notificationService) MarkAsRead(ctx context.Context, notificationID, userID int64) error {
	return s.repo.MarkAsRead(ctx, notificationID, userID)
}
