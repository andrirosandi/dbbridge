package main

import (
	"context"
	"dbbridge/internal/api"
	"dbbridge/internal/config"
	"dbbridge/internal/data"
	"dbbridge/internal/logger"
	"dbbridge/internal/service"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	// Drivers
	_ "github.com/alexbrainman/odbc"
	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func main() {
	// 1. Load Config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\nCheck .env file or DBBRIDGE_KEY environment variable.\n", err)
		os.Exit(1)
	}

	// 2. Initialize Logger
	logDir := "logs"
	if err := logger.Init(logDir); err != nil {
		fmt.Printf("Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	logger.Info.Println("Starting DbBridge...")

	// 3. Initialize DB
	db, err := data.InitDB()
	if err != nil {
		logger.Error.Fatalf("Failed to init database: %v", err)
	}
	defer db.Close()

	// 4. Initialize Repos
	connRepo := data.NewConnectionRepo(db)
	queryRepo := data.NewQueryRepo(db)
	// userRepo := data.NewUserRepo(db) // Not used directly in executor yet

	// 5. Initialize Services
	cryptoSvc, err := service.NewEncryptionService(cfg.DbBridgeKey)
	if err != nil {
		logger.Error.Fatalf("Failed to init crypto service: %v", err)
	}

	userRepo := data.NewUserRepo(db)
	apiKeyRepo := data.NewApiKeyRepo(db)
	authSvc := service.NewAuthService(userRepo, apiKeyRepo)
	auditRepo := data.NewAuditRepo(db)
	queryExecutor := service.NewQueryExecutor(connRepo, queryRepo, auditRepo, cryptoSvc)

	// 6. Initialize Handlers
	webHandler := api.NewWebHandler(connRepo, queryRepo, auditRepo, userRepo, apiKeyRepo, authSvc, cryptoSvc, cfg)
	authHandler := api.NewAuthHandler(authSvc, cfg.DbBridgeKey, webHandler.GetTemplates()) // Helper to share templates? Or just reuse.
	// To avoid circular dependency or change WebHandler, let's just expose Templates field in WebHandler or pass nil if AuthHandler parses its own?
	// Better: Init templates in main and pass to both. But WebHandler has logic.
	// I'll update NewAuthHandler usage to assume it shares templates if possible or loads same.
	// For simplicity in this edit block (limit changes), I will let NewAuthHandler() parse its own as implemented above, unless I change it.
	// Wait, I implemented NewAuthHandler accepting templates.
	// Let's add a getter to WebHandler or make templates public field.
	// Modifying WebHandler in main:

	// 7. Initialize Handlers
	docHandler := api.NewDocHandler(queryRepo, connRepo)
	apiHandler := api.NewHandler(queryExecutor, docHandler, authSvc)

	// 7. Start Server
	r := chi.NewRouter()
	r.Use(api.LoggingMiddleware)

	// Public Routes
	r.Get("/setup", authHandler.SetupPage)
	r.Post("/setup", authHandler.DoSetup)
	r.Get("/login", authHandler.LoginPage)
	r.Post("/login", authHandler.DoLogin)
	r.Get("/logout", authHandler.Logout)

	// Protected Admin Routes
	r.Group(func(r chi.Router) {
		r.Use(authHandler.AdminMiddleware)
		webHandler.RegisterRoutes(r)
	})

	// Public API (Protected by API Key inside handler/middleware but we might want to standardize)
	// API currently uses its own AuthMiddleware placeholder.
	r.Mount("/api", apiHandler.Routes()) // /api/...

	// Static files (Public)
	webHandler.RegisterStatic(r)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}

	// Graceful shutdown channel
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info.Printf("Server listening on port %d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error.Fatalf("Server startup failed: %v", err)
		}
	}()

	<-stop
	logger.Info.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error.Printf("Server shutdown error: %v", err)
	}
	logger.Info.Println("Server stopped")
}
