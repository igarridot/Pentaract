use std::{path::Path, sync::Arc};

use axum::{
    body::Full,
    extract::{DefaultBodyLimit, Multipart, Path as RoutePath, Query, State},
    http::StatusCode,
    middleware,
    response::{AppendHeaders, IntoResponse, Response},
    routing::{get, post},
    Extension, Json, Router,
};
use reqwest::header;
use tokio::io::AsyncWriteExt;
use tokio_util::bytes::Bytes;
use uuid::Uuid;

use crate::{
    common::{
        jwt_manager::AuthUser,
        routing::{app_state::AppState, middlewares::auth::logged_in_required},
    },
    errors::{PentaractError, PentaractResult},
    models::files::InFile,
    schemas::files::{InFileSchema, InFolderSchema, SearchQuery, UploadParams},
    services::files::FilesService,
};

pub struct FilesRouter;

impl FilesRouter {
    pub fn get_router(state: Arc<AppState>) -> Router<Arc<AppState>, axum::body::Body> {
        Router::new()
            .route("/create_folder", post(Self::create_folder))
            .route("/upload", post(Self::upload))
            .route("/upload_to", post(Self::upload_to))
            .route("/*path", get(Self::dynamic_get).delete(Self::delete))
            .layer(DefaultBodyLimit::disable())
            .route_layer(middleware::from_fn_with_state(
                state.clone(),
                logged_in_required,
            ))
            .with_state(state)
    }

    async fn dynamic_get(
        State(state): State<Arc<AppState>>,
        Extension(user): Extension<AuthUser>,
        RoutePath((storage_id, path)): RoutePath<(Uuid, String)>,
        query: Query<SearchQuery>,
    ) -> impl IntoResponse {
        let (root_path, path) = path.split_once("/").unwrap_or((&path, ""));
        match root_path {
            "tree" => Self::tree(state, user, storage_id, path).await,
            "download" => Self::download(state, user, storage_id, path).await,
            "search" => {
                if let Some(search_path) = query.0.search_path {
                    Self::search(state, user, storage_id, path, &search_path).await
                } else {
                    Err((
                        StatusCode::UNPROCESSABLE_ENTITY,
                        "search_path query parameter is required".to_owned(),
                    ))
                }
            }
            _ => Err((StatusCode::NOT_FOUND, "Not found".to_owned())),
        }
    }

    async fn tree(
        state: Arc<AppState>,
        user: AuthUser,
        storage_id: Uuid,
        path: &str,
    ) -> Result<Response, (StatusCode, String)> {
        let fs_layer = FilesService::new(&state.db, state.tx.clone())
            .list_dir(storage_id, path, &user)
            .await?;
        Ok(Json(fs_layer).into_response())
    }

    async fn upload(
        State(state): State<Arc<AppState>>,
        Extension(user): Extension<AuthUser>,
        RoutePath(storage_id): RoutePath<Uuid>,
        mut multipart: Multipart,
    ) -> Result<StatusCode, (StatusCode, String)> {
        // Ensure temp directory exists
        let temp_dir = &state.config.temp_dir;
        tokio::fs::create_dir_all(temp_dir).await.map_err(|e| {
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("Failed to create temp directory: {}", e),
            )
        })?;

        // Generate unique temp file path
        let temp_file_path = temp_dir.join(format!("upload_{}.tmp", Uuid::new_v4()));

        // parsing - stream file to disk instead of loading into memory
        let (path, size) = {
            let (mut filename, mut path, mut size) = (None, None, 0i64);

            while let Some(mut field) = multipart.next_field().await.map_err(|e| {
                (StatusCode::BAD_REQUEST, format!("Multipart error: {}", e))
            })? {
                let name = field.name().unwrap_or("").to_owned();
                let field_filename = field.file_name().unwrap_or("unnamed").to_owned();

                match name.as_str() {
                    "file" => {
                        filename = Some(field_filename);
                        // Stream file data to disk
                        let mut temp_file =
                            tokio::fs::File::create(&temp_file_path).await.map_err(|e| {
                                (
                                    StatusCode::INTERNAL_SERVER_ERROR,
                                    format!("Failed to create temp file: {}", e),
                                )
                            })?;

                        while let Some(chunk) = field.chunk().await.map_err(|e| {
                            (StatusCode::BAD_REQUEST, format!("Failed to read chunk: {}", e))
                        })? {
                            size += chunk.len() as i64;
                            temp_file.write_all(&chunk).await.map_err(|e| {
                                (
                                    StatusCode::INTERNAL_SERVER_ERROR,
                                    format!("Failed to write to temp file: {}", e),
                                )
                            })?;
                        }

                        temp_file.flush().await.map_err(|e| {
                            (
                                StatusCode::INTERNAL_SERVER_ERROR,
                                format!("Failed to flush temp file: {}", e),
                            )
                        })?;
                    }
                    "path" => {
                        let data = field.bytes().await.map_err(|e| {
                            (StatusCode::BAD_REQUEST, format!("Failed to read path: {}", e))
                        })?;
                        path = Some(String::from_utf8(data.to_vec()).map_err(|_| {
                            (StatusCode::BAD_REQUEST, "Path is not valid UTF-8".to_owned())
                        })?);
                    }
                    _ => (),
                }
            }

            let filename =
                filename.ok_or((StatusCode::BAD_REQUEST, "file field is required".to_owned()))?;
            let path = path
                .ok_or((StatusCode::BAD_REQUEST, "path field is required".to_owned()))
                .map(|path| Self::construct_path(&path, &filename))??;
            (path, size)
        };

        let in_file = InFile::new(path, size, storage_id);

        let result = FilesService::new(&state.db, state.tx.clone())
            .upload_anyway(in_file, temp_file_path.clone(), &user)
            .await;

        // Clean up temp file on error (success cleanup is handled by storage manager)
        if result.is_err() {
            let _ = tokio::fs::remove_file(&temp_file_path).await;
        }

        result?;
        Ok(StatusCode::CREATED)
    }

    async fn upload_to(
        State(state): State<Arc<AppState>>,
        Extension(user): Extension<AuthUser>,
        RoutePath(storage_id): RoutePath<Uuid>,
        mut multipart: Multipart,
    ) -> Result<StatusCode, (StatusCode, String)> {
        // Ensure temp directory exists
        let temp_dir = &state.config.temp_dir;
        tokio::fs::create_dir_all(temp_dir).await.map_err(|e| {
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                format!("Failed to create temp directory: {}", e),
            )
        })?;

        // Generate unique temp file path
        let temp_file_path = temp_dir.join(format!("upload_{}.tmp", Uuid::new_v4()));

        // parsing and validating schema - stream file to disk
        let in_schema = {
            let (mut path, mut size) = (None, 0i64);

            while let Some(mut field) = multipart.next_field().await.map_err(|e| {
                (StatusCode::BAD_REQUEST, format!("Multipart error: {}", e))
            })? {
                let name = field.name().unwrap_or("").to_owned();

                match name.as_str() {
                    "file" => {
                        // Stream file data to disk
                        let mut temp_file =
                            tokio::fs::File::create(&temp_file_path).await.map_err(|e| {
                                (
                                    StatusCode::INTERNAL_SERVER_ERROR,
                                    format!("Failed to create temp file: {}", e),
                                )
                            })?;

                        while let Some(chunk) = field.chunk().await.map_err(|e| {
                            (StatusCode::BAD_REQUEST, format!("Failed to read chunk: {}", e))
                        })? {
                            size += chunk.len() as i64;
                            temp_file.write_all(&chunk).await.map_err(|e| {
                                (
                                    StatusCode::INTERNAL_SERVER_ERROR,
                                    format!("Failed to write to temp file: {}", e),
                                )
                            })?;
                        }

                        temp_file.flush().await.map_err(|e| {
                            (
                                StatusCode::INTERNAL_SERVER_ERROR,
                                format!("Failed to flush temp file: {}", e),
                            )
                        })?;
                    }
                    "path" => {
                        let data = field.bytes().await.map_err(|e| {
                            (StatusCode::BAD_REQUEST, format!("Failed to read path: {}", e))
                        })?;
                        path = Some(String::from_utf8(data.to_vec()).map_err(|_| {
                            (StatusCode::BAD_REQUEST, "Path is not valid UTF-8".to_owned())
                        })?);
                    }
                    _ => (),
                }
            }

            let path =
                path.ok_or((StatusCode::BAD_REQUEST, "path field is required".to_owned()))?;

            if size == 0 {
                return Err((StatusCode::BAD_REQUEST, "file field is required".to_owned()));
            }

            InFileSchema::new(storage_id, path, size, temp_file_path.clone())
        };

        // do all other stuff
        let result = FilesService::new(&state.db, state.tx.clone())
            .upload_to(in_schema, &user)
            .await;

        // Clean up temp file on error (success cleanup is handled by storage manager)
        if result.is_err() {
            let _ = tokio::fs::remove_file(&temp_file_path).await;
        }

        result?;
        Ok(StatusCode::CREATED)
    }

    async fn create_folder(
        State(state): State<Arc<AppState>>,
        Extension(user): Extension<AuthUser>,
        RoutePath(storage_id): RoutePath<Uuid>,
        Json(params): Json<UploadParams>,
    ) -> Result<StatusCode, (StatusCode, String)> {
        let in_schema = InFolderSchema::new(storage_id, params.path, params.folder_name);

        FilesService::new(&state.db, state.tx.clone())
            .create_folder(in_schema, &user)
            .await?;
        Ok(StatusCode::CREATED)
    }

    #[inline]
    fn construct_path(path: &str, filename: &str) -> PentaractResult<String> {
        Path::new(path)
            .join(filename)
            .to_str()
            .ok_or(PentaractError::InvalidPath)
            .map(|p| p.to_string())
    }

    async fn download(
        state: Arc<AppState>,
        user: AuthUser,
        storage_id: Uuid,
        path: &str,
    ) -> Result<Response, (StatusCode, String)> {
        FilesService::new(&state.db, state.tx.clone())
            .download(path, storage_id, &user)
            .await
            .map(|data| {
                let filename = Path::new(&path)
                    .file_name()
                    .map(|name| name.to_str().unwrap_or_default())
                    .unwrap_or("unnamed.bin");
                let content_type = mime_guess::from_path(filename)
                    .first_or_octet_stream()
                    .to_string();
                let bytes = Bytes::from(data);
                let body = Full::new(bytes);

                let headers = AppendHeaders([
                    (header::CONTENT_TYPE, content_type),
                    (
                        header::CONTENT_DISPOSITION,
                        format!("attachment; filename=\"{filename}\""),
                    ),
                ]);

                (headers, body).into_response()
            })
            .map_err(|e| <(StatusCode, String)>::from(e))
    }

    ///
    /// Need path with trailing slash
    ///
    async fn search(
        state: Arc<AppState>,
        user: AuthUser,
        storage_id: Uuid,
        path: &str,
        search_path: &str,
    ) -> Result<Response, (StatusCode, String)> {
        FilesService::new(&state.db, state.tx.clone())
            .search(storage_id, path, search_path, &user)
            .await
            .map(|files| Json(files).into_response())
            .map_err(|e| <(StatusCode, String)>::from(e))
    }

    async fn delete(
        State(state): State<Arc<AppState>>,
        Extension(user): Extension<AuthUser>,
        RoutePath((storage_id, path)): RoutePath<(Uuid, String)>,
    ) -> Result<(), (StatusCode, String)> {
        FilesService::new(&state.db, state.tx.clone())
            .delete(&path, storage_id, &user)
            .await
            .map_err(|e| <(StatusCode, String)>::from(e))?;

        Ok(())
    }
}
