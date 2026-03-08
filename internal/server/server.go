package server

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
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
	storageManager := service.NewStorageManager(filesRepo, storagesRepo, scheduler, tgClient, cfg.SecretKey)

	authSvc := service.NewAuthService(usersRepo, cfg.SecretKey, cfg.AccessTokenExpireInSec)
	usersSvc := service.NewUsersService(usersRepo)
	storagesSvc := service.NewStoragesService(storagesRepo, accessRepo, filesRepo, storageManager)
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
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Authorization", "Content-Type"},
		MaxAge:         300,
	}))

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/login", authH.Login)
		r.Post("/users", usersH.Register)

		r.Group(func(r chi.Router) {
			r.Use(handler.AuthMiddleware(cfg.SecretKey))

			r.Get("/storages", storagesH.List)
			r.Post("/storages", storagesH.Create)
			r.Get("/storages/{storageID}", storagesH.Get)
			r.Delete("/storages/{storageID}", storagesH.Delete)

			r.Get("/storages/{storageID}/access", accessH.List)
			r.Post("/storages/{storageID}/access", accessH.Grant)
			r.Delete("/storages/{storageID}/access", accessH.Revoke)

			r.Get("/storage_workers", workersH.List)
			r.Post("/storage_workers", workersH.Create)
			r.Put("/storage_workers/{workerID}", workersH.Update)
			r.Delete("/storage_workers/{workerID}", workersH.Delete)
			r.Get("/storage_workers/has_workers", workersH.HasWorkers)

			r.Post("/storages/{storageID}/files/create_folder", filesH.CreateFolder)
			r.Post("/storages/{storageID}/files/move", filesH.Move)
			r.Post("/storages/{storageID}/files/upload", filesH.Upload)
			r.Get("/storages/{storageID}/files/tree/*", filesH.Tree)
			r.Get("/storages/{storageID}/files/download/*", filesH.Download)
			r.Get("/storages/{storageID}/files/download_dir/*", filesH.DownloadDir)
			r.Get("/storages/{storageID}/files/search/*", filesH.Search)
			r.Delete("/storages/{storageID}/files/*", filesH.DeleteFile)

			r.Get("/upload_progress", filesH.UploadProgress)
			r.Get("/download_progress", filesH.DownloadProgress)
			r.Get("/delete_progress", filesH.DeleteProgress)
			r.Post("/upload_cancel/{uploadID}", filesH.CancelUpload)
			r.Post("/download_cancel/{downloadID}", filesH.CancelDownload)
		})
	})

	// Serve frontend
	serveUI(r)

	return r
}

func serveUI(r chi.Router) {
	uiDir := "ui/dist"
	if _, err := os.Stat(uiDir); os.IsNotExist(err) {
		log.Println("UI directory not found, skipping static file serving")
		return
	}

	absUI, err := filepath.Abs(uiDir)
	if err != nil {
		log.Printf("Failed to resolve UI directory path: %v", err)
		return
	}
	uiFS := os.DirFS(absUI)
	indexFile := "index.html"

	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		requested := path.Clean("/" + r.URL.Path)
		requested = strings.TrimPrefix(requested, "/")

		if requested != "" && requested != "." && !strings.HasPrefix(requested, "..") {
			if info, statErr := fs.Stat(uiFS, requested); statErr == nil && info.Mode().IsRegular() {
				http.ServeFileFS(w, r, uiFS, requested)
				return
			}
		}

		http.ServeFileFS(w, r, uiFS, indexFile)
	})
}
