package routes

import (
	"github.com/Refliqx/backend-eletter/internal/handler"
	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/middleware"
	"github.com/gin-gonic/gin"
)

func SetupRouter(
	cfg *config.Config,
	authHandler *handler.AuthHandler,
	userProfileHandler *handler.UserProfileHandler,
	permissionHandler *handler.PermissionHandler,
	letterHandler *handler.LetterHandler,
	attachmentHandler *handler.AttachmentHandler,
) *gin.Engine {
	r := gin.New()

	r.Use(
		middleware.Logger(),
		gin.Recovery(),
		middleware.CORS(),
	)

	api := r.Group("/api/v1")
	{
		api.POST("/register", authHandler.Register)
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/logout", authHandler.Logout)
		api.POST("/auth/refresh", authHandler.Refresh)
		api.GET("/protected", authHandler.Protected)
		api.POST("/protected", authHandler.Protected)

		protected := api.Group("/")
		protected.Use(middleware.RequireAccessToken(cfg.JWT.Secret))
		{
			protected.GET("/user/profile", userProfileHandler.GetProfile)
			protected.POST("/user/profile", userProfileHandler.GetProfile)
			protected.POST("/user/update", userProfileHandler.UpdateProfile)

			protected.GET("/permission-requests", permissionHandler.GetRequests)
			protected.POST("/permission-requests", permissionHandler.CreateRequest)
			protected.PUT("/permission-requests", permissionHandler.UpdateRequest)
			protected.DELETE("/permission-requests", permissionHandler.DeleteRequest)
			protected.POST("/approve", permissionHandler.Approve)

			protected.POST("/letters/student/create", letterHandler.CreateStudent)
			protected.POST("/letters/teacher/create", letterHandler.CreateTeacher)

			protected.GET("/letters/student/izin-masuk", letterHandler.StudentIzinMasuk)
			protected.GET("/letters/student/izin-keluar", letterHandler.StudentIzinKeluar)
			protected.GET("/letters/student/dispensasi", letterHandler.StudentDispensasi)
			protected.GET("/letters/teacher/izin-masuk", letterHandler.TeacherIzinMasuk)
			protected.GET("/letters/teacher/izin-keluar", letterHandler.TeacherIzinKeluar)
			protected.GET("/letters/teacher/dispensasi", letterHandler.TeacherDispensasi)
		}

		api.GET("/attachments/:id", attachmentHandler.GetByID)
		api.GET("/attachments/request/:requestId", attachmentHandler.ListByRequest)

		api.GET("/dev/registration-token", permissionHandler.ListRegistrationTokens)
		api.POST("/dev/registration-token", permissionHandler.UpsertRegistrationToken)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"success": true,
			"message": "OK",
			"data":    nil,
		})
	})

	return r
}
