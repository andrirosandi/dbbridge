package main

import (
	"context"
	"dbbridge/internal/api"
	"dbbridge/internal/config"
	"dbbridge/internal/data"
	"dbbridge/internal/logger"
	"dbbridge/internal/service"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/term"

	// Drivers
	_ "github.com/alexbrainman/odbc"
	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func main() {
	// Check for CLI subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "reset-password":
			handleResetPassword(os.Args[2:])
			return
		case "help", "--help", "-h":
			printHelp()
			return
		default:
			fmt.Printf("Unknown command: %s\n", os.Args[1])
			printHelp()
			os.Exit(1)
		}
	}

	// No subcommand — start server
	startServer()
}

func printHelp() {
	fmt.Println("DbBridge - Database Bridge Server")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  dbbridge                         Start the server")
	fmt.Println("  dbbridge reset-password -u <user>  Reset user password (interactive)")
	fmt.Println("  dbbridge help                    Show this help")
}

func handleResetPassword(args []string) {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	username := fs.String("u", "", "Username to reset")
	fs.Parse(args)

	if *username == "" {
		fmt.Println("Usage: dbbridge reset-password -u <username>")
		os.Exit(1)
	}

	// Interactive password input (hidden)
	fmt.Print("New password: ")
	passBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // newline after hidden input
	if err != nil {
		fmt.Printf("Failed to read password: %v\n", err)
		os.Exit(1)
	}
	password := string(passBytes)

	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		fmt.Printf("Failed to read password: %v\n", err)
		os.Exit(1)
	}

	if password != string(confirmBytes) {
		fmt.Println("Passwords do not match.")
		os.Exit(1)
	}

	if password == "" {
		fmt.Println("Password cannot be empty.")
		os.Exit(1)
	}

	// Initialize minimal dependencies
	db, err := data.InitDB()
	if err != nil {
		fmt.Printf("Failed to init database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	userRepo := data.NewUserRepo(db)
	apiKeyRepo := data.NewApiKeyRepo(db)
	authSvc := service.NewAuthService(userRepo, apiKeyRepo)

	err = authSvc.ResetPassword(*username, password)
	if err != nil {
		fmt.Printf("Failed to reset password: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Password for user '%s' has been reset successfully.\n", *username)
}

func startServer() {
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
	authHandler := api.NewAuthHandler(authSvc, cfg.DbBridgeKey, webHandler.GetTemplates())

	docHandler := api.NewDocHandler(queryRepo, connRepo)
	apiHandler := api.NewHandler(queryExecutor, docHandler, authSvc)

	// 7. Start Server
	r := chi.NewRouter()
	r.Use(api.LoggingMiddleware)

	// Rate Limiters
	loginLimiter := api.NewRateLimiter(5, 3) // 5 req/min, burst 3 (brute force protection)
	apiLimiter := api.NewRateLimiter(60, 10) // 60 req/min, burst 10

	// Public Routes
	r.Get("/setup", authHandler.SetupPage)
	r.Post("/setup", authHandler.DoSetup)
	r.Get("/login", authHandler.LoginPage)
	r.With(loginLimiter.Middleware).Post("/login", authHandler.DoLogin)
	r.Get("/logout", authHandler.Logout)

	// Protected Admin Routes
	r.Group(func(r chi.Router) {
		r.Use(authHandler.AdminMiddleware)
		webHandler.RegisterRoutes(r)
	})

	// Public API (Protected by API Key + Rate Limiter)
	r.Route("/api", func(r chi.Router) {
		r.Use(apiLimiter.MiddlewareByAPIKey)
		r.Mount("/", apiHandler.Routes())
	})

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
