package routes

import (
	"github.com/Refliqx/backend-eletter/internal/config"
	"github.com/Refliqx/backend-eletter/internal/handler"
	"github.com/Refliqx/backend-eletter/internal/middleware"
	"github.com/gin-gonic/gin"
)

func SetupRouter(
	cfg *config.Config,
	rateLimiter *middleware.MultiRateLimiter,
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
		rateLimiter.GlobalRateLimit(),
	)

	r.Static("/uploads", "./public/uploads")
	r.Static("/signatures", "./public/uploads/signatures")

	api := r.Group("/api/v1")
	{
		// Public auth endpoints
		api.POST("/register", rateLimiter.RegisterRateLimiter(), authHandler.Register)
		api.POST("/auth/login", rateLimiter.LoginRateLimiter(), authHandler.Login)
		api.POST("/auth/admin-login", rateLimiter.LoginAdminRateLimiter(), authHandler.AdminLogin)
		api.POST("/auth/kepsek-login", rateLimiter.LoginAdminRateLimiter(), authHandler.KepsekLogin)
		api.POST("/auth/logout", authHandler.Logout)
		api.POST("/auth/refresh", rateLimiter.RefreshRateLimiter(), authHandler.Refresh)
		api.POST("/auth/forgot-password", rateLimiter.ForgotPasswordRateLimiter(), authHandler.ForgotPassword)
		api.POST("/auth/verify-otp", rateLimiter.VerifyOTPRateLimiter(), authHandler.VerifyOTP)
		api.POST("/auth/reset-password", rateLimiter.ResetPasswordRateLimiter(), authHandler.ResetPassword)
		api.GET("/protected", authHandler.Protected)
		api.POST("/protected", authHandler.Protected)
		api.GET("/config/school", adminHandler.GetSchoolConfig)

		// Protected routes (any authenticated user)
		protected := api.Group("/")
		protected.Use(middleware.RequireAccessToken(cfg.JWT.Secret))
		{
			// User profile
			protected.GET("/user/profile", rateLimiter.ReadRateLimiter(), userProfileHandler.GetProfile)
			protected.POST("/user/profile", rateLimiter.ReadRateLimiter(), userProfileHandler.GetProfile)
			protected.POST("/user/update", rateLimiter.WriteRateLimiter(), userProfileHandler.UpdateProfile)
			protected.POST("/user/signature", rateLimiter.WriteRateLimiter(), userProfileHandler.UploadSignature)
			protected.POST("/user/complete-onboarding", rateLimiter.WriteRateLimiter(), userProfileHandler.CompleteOnboarding)
			protected.GET("/user/schedules", rateLimiter.ReadRateLimiter(), userProfileHandler.GetSchedules)

			// Permission requests
			protected.GET("/permission-requests", rateLimiter.ReadRateLimiter(), permissionHandler.GetRequests)
			protected.POST("/permission-requests", rateLimiter.WriteRateLimiter(), permissionHandler.CreateRequest)
			protected.PUT("/permission-requests", rateLimiter.WriteRateLimiter(), permissionHandler.UpdateRequest)
			protected.DELETE("/permission-requests", rateLimiter.WriteRateLimiter(), permissionHandler.DeleteRequest)
			protected.POST("/permission-requests/:id/cancel", rateLimiter.WriteRateLimiter(), permissionHandler.CancelRequest)
			protected.GET("/permission-requests/:id/detail", rateLimiter.ReadRateLimiter(), permissionHandler.GetRequestDetail)
			protected.GET("/request-detail/:id", rateLimiter.ReadRateLimiter(), permissionHandler.GetRequestDetail)
			protected.POST("/approve", rateLimiter.WriteRateLimiter(), permissionHandler.Approve)

			// Letters
			protected.POST("/letters/student/create", rateLimiter.WriteRateLimiter(), letterHandler.CreateStudent)
			protected.POST("/letters/teacher/create", rateLimiter.WriteRateLimiter(), letterHandler.CreateTeacher)
			protected.POST("/letters/dispensasi", rateLimiter.WriteRateLimiter(), letterHandler.CreateTeacher)

			protected.GET("/letters/student/izin-masuk", rateLimiter.ReadRateLimiter(), letterHandler.StudentIzinMasuk)
			protected.GET("/letters/student/izin-keluar", rateLimiter.ReadRateLimiter(), letterHandler.StudentIzinKeluar)
			protected.GET("/letters/student/dispensasi", rateLimiter.ReadRateLimiter(), letterHandler.StudentDispensasi)
			protected.GET("/letters/teacher/izin-masuk", rateLimiter.ReadRateLimiter(), letterHandler.TeacherIzinMasuk)
			protected.GET("/letters/teacher/izin-keluar", rateLimiter.ReadRateLimiter(), letterHandler.TeacherIzinKeluar)
			protected.GET("/letters/teacher/dispensasi", rateLimiter.ReadRateLimiter(), letterHandler.TeacherDispensasi)
			protected.GET("/letters/teacher/pending", rateLimiter.ReadRateLimiter(), letterHandler.TeacherPending)
			protected.GET("/letters/teacher", rateLimiter.ReadRateLimiter(), letterHandler.TeacherLetters)
			protected.GET("/letters/dispensasi", rateLimiter.ReadRateLimiter(), letterHandler.GeneralDispensasi)
			protected.GET("/letters/general/dispensasi", rateLimiter.ReadRateLimiter(), letterHandler.GeneralDispensasi)
			protected.GET("/holidays", rateLimiter.ReadRateLimiter(), letterHandler.GetHolidays)
			protected.GET("/letters/kepsek/pending", rateLimiter.ReadRateLimiter(), letterHandler.KepsekPending)
			protected.GET("/letters/kepsek/stats", rateLimiter.ReadRateLimiter(), letterHandler.KepsekStats)

			// Attachments
			protected.GET("/attachments/:id", rateLimiter.ReadRateLimiter(), attachmentHandler.GetByID)
			protected.GET("/attachments/request/:requestId", rateLimiter.ReadRateLimiter(), attachmentHandler.ListByRequest)
			protected.POST("/attachments/upload", rateLimiter.WriteRateLimiter(), attachmentHandler.Upload)

			// Master data
			protected.GET("/classes", rateLimiter.ReadRateLimiter(), masterDataHandler.GetClasses)
			protected.GET("/class/:id", rateLimiter.ReadRateLimiter(), masterDataHandler.GetClass)
			protected.GET("/majors", rateLimiter.ReadRateLimiter(), masterDataHandler.GetMajors)
			protected.GET("/major/:id", rateLimiter.ReadRateLimiter(), masterDataHandler.GetMajor)
			protected.GET("/students", rateLimiter.ReadRateLimiter(), masterDataHandler.GetStudents)
			protected.GET("/subjects", rateLimiter.ReadRateLimiter(), adminHandler.GetSubjects)

			// Notifications
			protected.GET("/notifications", rateLimiter.ReadRateLimiter(), notificationHandler.GetNotifications)
			protected.PATCH("/notifications/:id/read", rateLimiter.WriteRateLimiter(), notificationHandler.MarkAsRead)

			// SSE
			protected.GET("/sse/events", rateLimiter.SSERateLimiter(), sseHandler.Stream)

			// Teacher-specific
			teacher := protected.Group("/teacher")
			teacher.Use(middleware.RequireRole("teacher"))
			{
				teacher.GET("/roles", rateLimiter.ReadRateLimiter(), permissionHandler.GetTeacherRoles)
				teacher.POST("/roles/request", rateLimiter.WriteRateLimiter(), permissionHandler.RequestTeacherRole)
				teacher.GET("/stats", rateLimiter.ReadRateLimiter(), letterHandler.TeacherStats)
				teacher.POST("/delegate", rateLimiter.WriteRateLimiter(), permissionHandler.CreateDelegation)
				teacher.GET("/delegates", rateLimiter.ReadRateLimiter(), permissionHandler.ListDelegations)
				teacher.DELETE("/delegate/:id", rateLimiter.WriteRateLimiter(), permissionHandler.DeleteDelegation)
			}

			// Admin-specific
			admin := protected.Group("/admin")
			admin.Use(middleware.RequireRole("admin"))
			admin.Use(rateLimiter.AdminRateLimiter())
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
				admin.POST("/config/upload", adminHandler.UploadConfigImage)
				admin.GET("/audit-logs", adminHandler.GetAuditLogs)
			}
		}

		// Dev endpoints
		api.GET("/dev/registration-token", rateLimiter.DevRateLimiter(), permissionHandler.ListRegistrationTokens)
		api.POST("/dev/registration-token", rateLimiter.DevRateLimiter(), permissionHandler.UpsertRegistrationToken)
	}

	healthHandler := func(c *gin.Context) {
		c.JSON(200, gin.H{"success": true, "message": "OK", "data": nil})
	}
	r.GET("/health", healthHandler)
	r.HEAD("/health", healthHandler)

	return r
}
