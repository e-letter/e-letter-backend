package routes

import (
	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/handler"
	"github.com/Refliqx/backend-eletter/internal/middleware"
	"github.com/gin-gonic/gin"
)

func SetupRouter(
	cfg *config.Config,
	rateLimiter *middleware.RedisRateLimiter,
	authHandler *handler.AuthHandler,
	userProfileHandler *handler.UserProfileHandler,
	permissionHandler *handler.PermissionHandler,
	letterHandler *handler.LetterHandler,
	attachmentHandler *handler.AttachmentHandler,
	masterDataHandler *handler.MasterDataHandler,
	notificationHandler *handler.NotificationHandler,
	adminHandler *handler.AdminHandler,
	sseHandler *handler.SSEHandler,
) *gin.Engine {
	r := gin.New()

	if len(cfg.App.TrustedProxies) > 0 {
		r.SetTrustedProxies(cfg.App.TrustedProxies)
	}

	r.Use(
		middleware.Logger(cfg.App.Env),
		gin.Recovery(),
		middleware.CORS(),
	)

	r.Static("/uploads", "./public/uploads")
	r.Static("/signatures", "./public/uploads/signatures")

	api := r.Group("/api/v1")
	{
		// Public auth endpoints
		api.POST("/register", authHandler.Register)
		api.POST("/auth/login", rateLimiter.LoginRateLimiter(), authHandler.Login)
		api.POST("/auth/admin-login", rateLimiter.LoginRateLimiter(), authHandler.AdminLogin)
		api.POST("/auth/kepsek-login", rateLimiter.LoginRateLimiter(), authHandler.KepsekLogin)
		api.POST("/auth/logout", authHandler.Logout)
		api.POST("/auth/refresh", authHandler.Refresh)
		api.POST("/auth/forgot-password", authHandler.ForgotPassword)
		api.POST("/auth/verify-otp", authHandler.VerifyOTP)
		api.POST("/auth/reset-password", authHandler.ResetPassword)
		api.GET("/protected", authHandler.Protected)
		api.POST("/protected", authHandler.Protected)
		api.GET("/config/school", adminHandler.GetSchoolConfig)

		// Protected routes (any authenticated user)
		protected := api.Group("/")
		protected.Use(middleware.RequireAccessToken(cfg.JWT.Secret))
		{
			// User profile
			protected.GET("/user/profile", userProfileHandler.GetProfile)
			protected.POST("/user/profile", userProfileHandler.GetProfile)
			protected.POST("/user/update", userProfileHandler.UpdateProfile)
			protected.POST("/user/signature", userProfileHandler.UploadSignature)
			protected.POST("/user/complete-onboarding", userProfileHandler.CompleteOnboarding)

			// Permission requests
			protected.GET("/permission-requests", permissionHandler.GetRequests)
			protected.POST("/permission-requests", permissionHandler.CreateRequest)
			protected.PUT("/permission-requests", permissionHandler.UpdateRequest)
			protected.DELETE("/permission-requests", permissionHandler.DeleteRequest)
			protected.POST("/permission-requests/:id/cancel", permissionHandler.CancelRequest)
			protected.GET("/permission-requests/:id/detail", permissionHandler.GetRequestDetail)
			protected.POST("/approve", permissionHandler.Approve)

			// Letters
			protected.POST("/letters/student/create", letterHandler.CreateStudent)
			protected.POST("/letters/teacher/create", letterHandler.CreateTeacher)
			protected.POST("/letters/dispensasi", letterHandler.CreateTeacher)

			protected.GET("/letters/student/izin-masuk", letterHandler.StudentIzinMasuk)
			protected.GET("/letters/student/izin-keluar", letterHandler.StudentIzinKeluar)
			protected.GET("/letters/student/dispensasi", letterHandler.StudentDispensasi)
			protected.GET("/letters/teacher/izin-masuk", letterHandler.TeacherIzinMasuk)
			protected.GET("/letters/teacher/izin-keluar", letterHandler.TeacherIzinKeluar)
			protected.GET("/letters/teacher/dispensasi", letterHandler.TeacherDispensasi)
			protected.GET("/letters/teacher/pending", letterHandler.TeacherPending)
			protected.GET("/letters/teacher", letterHandler.TeacherLetters)
			protected.GET("/letters/dispensasi", letterHandler.GeneralDispensasi)
			protected.GET("/letters/general/dispensasi", letterHandler.GeneralDispensasi)
			protected.GET("/letters/kepsek/pending", letterHandler.KepsekPending)
			protected.GET("/letters/kepsek/stats", letterHandler.KepsekStats)

			// Attachments
			protected.GET("/attachments/:id", attachmentHandler.GetByID)
			protected.GET("/attachments/request/:requestId", attachmentHandler.ListByRequest)
			protected.POST("/attachments/upload", attachmentHandler.Upload)

			// Master data
			protected.GET("/classes", masterDataHandler.GetClasses)
			protected.GET("/class/:id", masterDataHandler.GetClass)
			protected.GET("/majors", masterDataHandler.GetMajors)
			protected.GET("/major/:id", masterDataHandler.GetMajor)
			protected.GET("/students", masterDataHandler.GetStudents)
			protected.GET("/subjects", adminHandler.GetSubjects)

			// Notifications
			protected.GET("/notifications", notificationHandler.GetNotifications)
			protected.PATCH("/notifications/:id/read", notificationHandler.MarkAsRead)

			// SSE
			protected.GET("/sse/events", sseHandler.Stream)

			// Teacher-specific
			teacher := protected.Group("/teacher")
			teacher.Use(middleware.RequireRole("teacher"))
			{
				teacher.GET("/roles", permissionHandler.GetTeacherRoles)
				teacher.POST("/roles/request", permissionHandler.RequestTeacherRole)
				teacher.GET("/stats", letterHandler.TeacherStats)
				teacher.POST("/delegate", permissionHandler.CreateDelegation)
				teacher.GET("/delegates", permissionHandler.ListDelegations)
				teacher.DELETE("/delegate/:id", permissionHandler.DeleteDelegation)
			}

			// Admin-specific
			admin := protected.Group("/admin")
			admin.Use(middleware.RequireRole("admin"))
			{
				admin.GET("/users", adminHandler.GetUsers)
				admin.PATCH("/users/:id/status", adminHandler.UpdateUserStatus)
				admin.GET("/stats", adminHandler.GetStats)
				admin.GET("/registration-tokens", adminHandler.GetRegistrationTokens)
				admin.POST("/registration-tokens", adminHandler.CreateRegistrationToken)
				admin.DELETE("/registration-tokens/:id", adminHandler.DeleteRegistrationToken)
				admin.GET("/teacher-roles", adminHandler.ListPendingTeacherRoles)
				admin.PATCH("/teacher-roles/:id/verify", adminHandler.VerifyTeacherRole)
				admin.PATCH("/teacher-roles/:id/reject", adminHandler.RejectTeacherRole)
				admin.GET("/academic-years", adminHandler.GetAcademicYears)
				admin.POST("/academic-years", adminHandler.CreateAcademicYear)
				admin.PUT("/academic-years/:id", adminHandler.UpdateAcademicYear)
				admin.DELETE("/academic-years/:id", adminHandler.DeleteAcademicYear)
				admin.GET("/classes", adminHandler.GetClasses)
				admin.POST("/classes", adminHandler.CreateClass)
				admin.PUT("/classes/:id", adminHandler.UpdateClass)
				admin.DELETE("/classes/:id", adminHandler.DeleteClass)
				admin.GET("/majors", adminHandler.GetMajors)
				admin.POST("/majors", adminHandler.CreateMajor)
				admin.PUT("/majors/:id", adminHandler.UpdateMajor)
				admin.DELETE("/majors/:id", adminHandler.DeleteMajor)
				admin.GET("/subjects", adminHandler.GetSubjects)
				admin.POST("/subjects", adminHandler.CreateSubject)
				admin.PUT("/subjects/:id", adminHandler.UpdateSubject)
				admin.DELETE("/subjects/:id", adminHandler.DeleteSubject)
				admin.GET("/enrollments", adminHandler.GetEnrollments)
				admin.POST("/enrollments", adminHandler.CreateEnrollment)
				admin.DELETE("/enrollments/:id", adminHandler.DeleteEnrollment)
				admin.PUT("/config/school", adminHandler.UpdateSchoolConfig)
				admin.GET("/audit-logs", adminHandler.GetAuditLogs)
			}
		}

		// Dev endpoints
		api.GET("/dev/registration-token", permissionHandler.ListRegistrationTokens)
		api.POST("/dev/registration-token", permissionHandler.UpsertRegistrationToken)
	}

	healthHandler := func(c *gin.Context) {
		c.JSON(200, gin.H{"success": true, "message": "OK", "data": nil})
	}
	r.GET("/health", healthHandler)
	r.HEAD("/health", healthHandler)

	return r
}
