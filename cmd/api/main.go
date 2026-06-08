package main

import (
	"log"
	"time"

	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/handler"
	"github.com/Refliqx/backend-eletter/internal/mailer"
	"github.com/Refliqx/backend-eletter/internal/middleware"
	"github.com/Refliqx/backend-eletter/internal/repository"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/Refliqx/backend-eletter/routes"
)

func main() {
	cfg := config.LoadConfig()

	time.Local = time.FixedZone("WIB", 7*60*60)

	// Verify the configured timezone is loadable; warn if not.
	if _, err := time.LoadLocation(cfg.App.Timezone); err != nil {
		log.Printf("Warning: timezone %s not found in IANA database", cfg.App.Timezone)
	}

	db := config.NewMySQLDB(cfg)

	config.RunAutoMigrate(db)

	rateLimiter := middleware.NewMultiRateLimiter(cfg)
	defer rateLimiter.Close()

	eventBus := handler.NewEventBus()
	sseHandler := handler.NewSSEHandler(eventBus)

	authRepo := repository.NewAuthRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)

	emailMailer := mailer.New(mailer.Config{
		APIKey: cfg.Email.APIKey,
	})

	authService := service.NewAuthService(authRepo, notificationRepo, emailMailer, cfg.JWT.Secret, cfg.JWT.AccessExpiresIn, cfg.JWT.RefreshExpiresIn)
	authHandler := handler.NewAuthHandler(authService, cfg, rateLimiter, db)

	userProfileRepo := repository.NewUserProfileRepository(db)
	userProfileService := service.NewUserProfileService(userProfileRepo)
	userProfileHandler := handler.NewUserProfileHandler(userProfileService, cfg.App.BaseURL, db)

	permissionRepo := repository.NewPermissionRepository(db, cfg.App.SchoolCode, eventBus)
	permissionService := service.NewPermissionService(permissionRepo)
	permissionHandler := handler.NewPermissionHandler(permissionService, cfg.App.Env != "production", db)

	letterRepo := repository.NewLetterRepository(db, cfg.App.SchoolCode, eventBus)
	letterService := service.NewLetterService(letterRepo, cfg.App.BaseURL)
	letterHandler := handler.NewLetterHandler(letterService, db)

	attachmentRepo := repository.NewAttachmentRepository(db)
	attachmentService := service.NewAttachmentService(attachmentRepo)
	attachmentHandler := handler.NewAttachmentHandler(attachmentService)

	masterDataRepo := repository.NewMasterDataRepository(db)
	masterDataService := service.NewMasterDataService(masterDataRepo)
	masterDataHandler := handler.NewMasterDataHandler(masterDataService)

	notificationService := service.NewNotificationService(notificationRepo)
	notificationHandler := handler.NewNotificationHandler(notificationService)

	adminHandler := handler.NewAdminHandler(db)

	router := routes.SetupRouter(
		cfg,
		rateLimiter,
		authHandler,
		userProfileHandler,
		permissionHandler,
		letterHandler,
		attachmentHandler,
		masterDataHandler,
		notificationHandler,
		adminHandler,
		sseHandler,
	)

	log.Printf("Server running in %s mode on port %s\n", cfg.App.Env, cfg.App.Port)

	if err := router.Run(":" + cfg.App.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
