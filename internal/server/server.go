package server

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/config"
	"github.com/Dominux/Pentaract/internal/handler"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/Dominux/Pentaract/internal/service"
	"github.com/Dominux/Pentaract/internal/telegram"
)

func New(cfg *config.Config, pool *pgxpool.Pool) http.Handler {
	// Repositories
	usersRepo := repository.NewUsersRepo(pool)
	storagesRepo := repository.NewStoragesRepo(pool)
	accessRepo := repository.NewAccessRepo(pool)
	workersRepo := repository.NewStorageWorkersRepo(pool)
	filesRepo := repository.NewFilesRepo(pool)

	// Telegram client
	tgClient := telegram.NewClient(cfg.TelegramAPIBaseURL)

	// Services
	scheduler := service.NewWorkerScheduler(workersRepo, cfg.TelegramRateLimit)
	storageManager := service.NewStorageManager(filesRepo, storagesRepo, scheduler, tgClient)

	authSvc := service.NewAuthService(usersRepo, cfg.SecretKey, cfg.AccessTokenExpireInSec)
	usersSvc := service.NewUsersService(usersRepo)
	storagesSvc := service.NewStoragesService(storagesRepo, accessRepo)
	accessSvc := service.NewAccessService(accessRepo, usersRepo)
	workersSvc := service.NewStorageWorkersService(workersRepo)
	filesSvc := service.NewFilesService(filesRepo, accessRepo, storageManager)

	// Handlers
	authH := handler.NewAuthHandler(authSvc)
	usersH := handler.NewUsersHandler(usersSvc)
	storagesH := handler.NewStoragesHandler(storagesSvc)
	accessH := handler.NewAccessHandler(accessSvc)
	workersH := handler.NewStorageWorkersHandler(workersSvc)
	filesH := handler.NewFilesHandler(filesSvc)

	// Router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Throttle(cfg.Workers))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Public routes
		r.Post("/auth/login", authH.Login)
		r.Post("/users", usersH.Register)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(handler.AuthMiddleware(cfg.SecretKey))

			// Storages
			r.Get("/storages", storagesH.List)
			r.Post("/storages", storagesH.Create)
			r.Get("/storages/{storageID}", storagesH.Get)
			r.Delete("/storages/{storageID}", storagesH.Delete)

			// Access
			r.Get("/storages/{storageID}/access", accessH.List)
			r.Post("/storages/{storageID}/access", accessH.Grant)
			r.Delete("/storages/{storageID}/access", accessH.Revoke)

			// Storage workers
			r.Get("/storage_workers", workersH.List)
			r.Post("/storage_workers", workersH.Create)
			r.Delete("/storage_workers/{workerID}", workersH.Delete)
			r.Get("/storage_workers/has_workers", workersH.HasWorkers)

			// Files
			r.Post("/storages/{storageID}/files/create_folder", filesH.CreateFolder)
			r.Post("/storages/{storageID}/files/upload", filesH.Upload)
			r.Post("/storages/{storageID}/files/upload_to", filesH.UploadTo)
			r.Get("/storages/{storageID}/files/tree/*", filesH.Tree)
			r.Get("/storages/{storageID}/files/download/*", filesH.Download)
			r.Get("/storages/{storageID}/files/search/*", filesH.Search)
			r.Delete("/storages/{storageID}/files/*", filesH.DeleteFile)

			// Upload progress (SSE) and cancel
			r.Get("/upload_progress", filesH.UploadProgress)
			r.Post("/upload_cancel/{uploadID}", filesH.CancelUpload)
		})
	})

	// Serve frontend static files
	serveUI(r)

	return r
}

func serveUI(r chi.Router) {
	uiDir := "ui/dist"

	// Check if UI directory exists
	if _, err := os.Stat(uiDir); os.IsNotExist(err) {
		log.Println("UI directory not found, skipping static file serving")
		return
	}

	// Serve static assets
	fileServer(r, "/assets", http.Dir(filepath.Join(uiDir, "assets")))

	// SPA fallback: serve index.html for all non-API, non-asset routes
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve static file first
		path := filepath.Join(uiDir, r.URL.Path)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		// Fallback to index.html for SPA routing
		http.ServeFile(w, r, filepath.Join(uiDir, "index.html"))
	})
}

func fileServer(r chi.Router, path string, root http.FileSystem) {
	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fsHandler := http.StripPrefix(pathPrefix, http.FileServer(root))
		fsHandler.ServeHTTP(w, r)
	})

	_ = fs.FS(nil) // ensure fs import is used
}
