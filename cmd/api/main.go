package main

import (
	"log"

	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/handler"
	"github.com/Refliqx/backend-eletter/internal/repository"
	"github.com/Refliqx/backend-eletter/internal/service"
	"github.com/Refliqx/backend-eletter/routes"
)

func main() {
	cfg := config.LoadConfig()

	// init DB
	db := config.NewPostgresDB(cfg)

	authRepo := repository.NewAuthRepository(db)
	authService := service.NewAuthService(authRepo, cfg.JWT.Secret, cfg.JWT.AccessExpiresIn, cfg.JWT.RefreshExpiresIn)
	authHandler := handler.NewAuthHandler(authService)

	userProfileRepo := repository.NewUserProfileRepository(db)
	userProfileService := service.NewUserProfileService(userProfileRepo)
	userProfileHandler := handler.NewUserProfileHandler(userProfileService)

	permissionRepo := repository.NewPermissionRepository(db)
	permissionService := service.NewPermissionService(permissionRepo)
	permissionHandler := handler.NewPermissionHandler(permissionService, cfg.App.Env != "production")

	letterRepo := repository.NewLetterRepository(db)
	letterService := service.NewLetterService(letterRepo)
	letterHandler := handler.NewLetterHandler(letterService)

	attachmentRepo := repository.NewAttachmentRepository(db)
	attachmentService := service.NewAttachmentService(attachmentRepo)
	attachmentHandler := handler.NewAttachmentHandler(attachmentService)

	router := routes.SetupRouter(
		cfg,
		authHandler,
		userProfileHandler,
		permissionHandler,
		letterHandler,
		attachmentHandler,
	)

	log.Printf("Server running in %s mode on port %s\n", cfg.App.Env, cfg.App.Port)

	if err := router.Run(":" + cfg.App.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
