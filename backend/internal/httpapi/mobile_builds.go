package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"tabel/internal/repo"
)

const maxMobileBuildSize = 500 << 20

func mobileBuildsDir() string {
	if value := strings.TrimSpace(os.Getenv("MOBILE_BUILDS_DIR")); value != "" {
		return value
	}
	return "/data/mobile"
}

func (s *Server) handleListMobileBuilds(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	builds, err := rp.ListMobileBuilds(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить список сборок")
		return
	}
	if builds == nil {
		builds = []repo.MobileBuild{}
	}
	writeJSON(w, http.StatusOK, builds)
}

func (s *Server) handleUploadMobileBuild(w http.ResponseWriter, r *http.Request) {
	principal, _ := principalFrom(r.Context())
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	if err := r.ParseMultipartForm(maxMobileBuildSize); err != nil {
		writeError(w, http.StatusBadRequest, "не удалось прочитать форму")
		return
	}
	platform := strings.TrimSpace(r.FormValue("platform"))
	version := strings.TrimSpace(r.FormValue("version"))
	if platform != "android" && platform != "ios" {
		writeError(w, http.StatusBadRequest, "platform должен быть android или ios")
		return
	}
	if version == "" {
		writeError(w, http.StatusBadRequest, "version обязательна")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "файл обязателен (поле file)")
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if platform == "android" && ext != ".apk" {
		writeError(w, http.StatusBadRequest, "для Android ожидается файл .apk")
		return
	}
	if platform == "ios" && ext != ".ipa" {
		writeError(w, http.StatusBadRequest, "для iOS ожидается файл .ipa")
		return
	}
	dir := mobileBuildsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось подготовить хранилище")
		return
	}
	storedName := uuid.NewString() + ext
	storedPath := filepath.Join(dir, storedName)
	target, err := os.Create(storedPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить файл")
		return
	}
	size, err := io.Copy(target, io.LimitReader(file, maxMobileBuildSize+1))
	closeErr := target.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(storedPath)
		writeError(w, http.StatusInternalServerError, "не удалось сохранить файл")
		return
	}
	if size > maxMobileBuildSize {
		_ = os.Remove(storedPath)
		writeError(w, http.StatusRequestEntityTooLarge, "файл слишком большой")
		return
	}
	uploadedBy := principal.UserID
	build, err := rp.CreateMobileBuild(r.Context(), platform, version, header.Filename, storedPath, size, &uploadedBy)
	if err != nil {
		_ = os.Remove(storedPath)
		writeError(w, http.StatusInternalServerError, "не удалось зарегистрировать сборку")
		return
	}
	s.audit(r, rp, &principal.UserID, "mobile_build.upload", "mobile_build", build.ID, map[string]any{
		"platform": platform, "version": version, "size": size,
	})
	writeJSON(w, http.StatusCreated, build)
}

func (s *Server) handleDeleteMobileBuild(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	build, err := rp.GetMobileBuild(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "сборка не найдена")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сборки")
		return
	}
	if err := rp.DeleteMobileBuild(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось удалить сборку")
		return
	}
	if build.StoredPath != "" {
		_ = os.Remove(build.StoredPath)
	}
	principal, _ := principalFrom(r.Context())
	s.audit(r, rp, &principal.UserID, "mobile_build.delete", "mobile_build", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePublicMobileBuilds(w http.ResponseWriter, r *http.Request) {
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	builds, err := rp.ListMobileBuilds(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить список сборок")
		return
	}
	result := make([]map[string]any, 0, len(builds))
	for _, build := range builds {
		if !build.IsActive {
			continue
		}
		result = append(result, map[string]any{
			"id":         build.ID,
			"platform":   build.Platform,
			"version":    build.Version,
			"filename":   build.Filename,
			"size_bytes": build.SizeBytes,
			"created_at": build.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleDownloadMobileBuild(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	rp, ok := s.repo()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "система не настроена")
		return
	}
	build, err := rp.GetMobileBuild(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) || !build.IsActive {
		writeError(w, http.StatusNotFound, "сборка не найдена")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка чтения сборки")
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename="+build.Filename)
	http.ServeFile(w, r, build.StoredPath)
}
