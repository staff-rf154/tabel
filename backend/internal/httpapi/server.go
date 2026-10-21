package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"tabel/internal/auth"
	"tabel/internal/config"
	"tabel/internal/qr"
	"tabel/internal/repo"
	"tabel/internal/settings"
)

type Server struct {
	mu          sync.RWMutex
	migrationMu sync.Mutex
	cfg         config.Config
	store       *settings.Store
	pool        *pgxpool.Pool
	rdb         *redis.Client
	tokens      *auth.TokenManager
	qrMgr       *qr.Manager
}

func NewServer(cfg config.Config, store *settings.Store, pool *pgxpool.Pool, rdb *redis.Client) *Server {
	secret := cfg.JWTSecret
	if secret == "" {
		sum := sha256.Sum256([]byte("tabel-jwt:" + cfg.EncryptionKey))
		secret = hex.EncodeToString(sum[:])
	}
	return &Server{
		cfg:    cfg,
		store:  store,
		pool:   pool,
		rdb:    rdb,
		tokens: auth.NewTokenManager([]byte(secret), cfg.JWTAccessTTL, cfg.JWTRefreshTTL),
		qrMgr:  qr.NewManager(store),
	}
}

// EnsureQRKey гарантирует, что ключ подписи QR создан к моменту запуска.
func (s *Server) EnsureQRKey() error {
	_, err := s.qrMgr.PublicKeyPEM()
	return err
}

func (s *Server) repo() (*repo.Repo, bool) {
	pool, _ := s.connections()
	if pool == nil {
		return nil, false
	}
	return repo.New(pool), true
}

func (s *Server) SetConnections(pool *pgxpool.Pool, rdb *redis.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pool = pool
	s.rdb = rdb
}

func (s *Server) connections() (*pgxpool.Pool, *redis.Client) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pool, s.rdb
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)
	mux.HandleFunc("GET /setup/status", s.handleSetupStatus)
	mux.HandleFunc("POST /setup/test-db", s.handleSetupTestDB)
	mux.HandleFunc("POST /setup/save", s.handleSetupSave)
	mux.HandleFunc("POST /setup/complete", s.handleSetupComplete)

	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/refresh", s.handleRefresh)
	mux.HandleFunc("POST /auth/register", s.handleRegister)
	mux.HandleFunc("POST /auth/logout", s.authenticated(s.handleLogout))
	mux.HandleFunc("POST /auth/password", s.authenticated(s.handleChangePassword))
	mux.HandleFunc("GET /me", s.authenticated(s.handleMe))
	mux.HandleFunc("GET /me/monthly-stats", s.authenticated(s.handleMyMonthlyStats))
	mux.HandleFunc("GET /mobile/bootstrap", s.authenticated(s.handleMobileBootstrap))

	mux.HandleFunc("POST /mark", s.authenticated(s.handleCreateMark))
	mux.HandleFunc("GET /marks", s.authenticated(s.handleListMarks))
	mux.HandleFunc("GET /qr/{token}", s.authenticated(s.handleVerifyQR))
	mux.HandleFunc("GET /schedules", s.authenticated(s.handleGetMySchedule))
	mux.HandleFunc("GET /me/qr", s.authenticated(s.handleMyQR))
	mux.HandleFunc("POST /me/qr/rotate", s.authenticated(s.handleRotateMyQR))
	mux.HandleFunc("GET /me/qr.png", s.authenticated(s.handleMyQRImage))

	mux.HandleFunc("GET /admin/employees", s.admin(s.handleListEmployees))
	mux.HandleFunc("POST /admin/employees", s.admin(s.handleCreateEmployee))
	mux.HandleFunc("GET /admin/employees/{id}", s.admin(s.handleGetEmployee))
	mux.HandleFunc("GET /admin/employees/{id}/qr", s.admin(s.handleAdminGetEmployeeQR))
	mux.HandleFunc("GET /admin/employees/{id}/qr.png", s.admin(s.handleAdminEmployeeQRImage))
	mux.HandleFunc("POST /admin/employees/{id}/qr/rotate", s.admin(s.handleAdminRotateEmployeeQR))
	mux.HandleFunc("PATCH /admin/employees/{id}", s.admin(s.handleUpdateEmployee))
	mux.HandleFunc("GET /admin/users", s.admin(s.handleListUsers))
	mux.HandleFunc("POST /admin/users/{id}/block", s.admin(s.handleBlockUser))
	mux.HandleFunc("PATCH /admin/users/{id}/role", s.admin(s.handleUpdateUserRole))
	mux.HandleFunc("PATCH /admin/users/{id}/password", s.admin(s.handleSetUserPassword))
	mux.HandleFunc("DELETE /admin/users/{id}", s.admin(s.handleDeleteUser))
	mux.HandleFunc("POST /admin/users/{id}/unblock", s.admin(s.handleUnblockUser))
	mux.HandleFunc("POST /admin/users/{id}/reset-registration", s.admin(s.handleResetRegistration))
	mux.HandleFunc("GET /admin/profile", s.admin(s.handleGetAdminProfile))
	mux.HandleFunc("PUT /admin/profile", s.admin(s.handleUpdateAdminProfile))
	mux.HandleFunc("GET /admin/system-settings", s.admin(s.handleGetSystemSettings))
	mux.HandleFunc("PUT /admin/system-settings", s.admin(s.handleUpdateSystemSettings))
	mux.HandleFunc("POST /admin/system-settings/test", s.admin(s.handleTestSystemConnections))
	mux.HandleFunc("POST /admin/system-settings/test-ssl", s.admin(s.handleTestSSL))
	mux.HandleFunc("POST /admin/system-settings/test-s3", s.admin(s.handleTestS3))
	mux.HandleFunc("GET /admin/system-settings/db-scripts", s.admin(s.handleListDBScripts))
	mux.HandleFunc("GET /admin/system-settings/db-scripts.zip", s.admin(s.handleDownloadDBScriptsZip))
	mux.HandleFunc("GET /admin/system-settings/db-scripts/{name}", s.admin(s.handleDownloadDBScript))
	mux.HandleFunc("GET /admin/system-settings/settings-db", s.admin(s.handleDownloadSettingsDB))
	mux.HandleFunc("POST /admin/system-settings/migrate-postgres", s.admin(s.handleMigratePostgreSQL))
	mux.HandleFunc("GET /admin/audit", s.admin(s.handleListAudit))
	mux.HandleFunc("GET /admin/marks", s.admin(s.handleListAdminMarks))
	mux.HandleFunc("GET /admin/schedules", s.admin(s.handleListSchedules))
	mux.HandleFunc("POST /admin/schedules", s.admin(s.handleCreateSchedule))
	mux.HandleFunc("GET /admin/schedule-templates", s.admin(s.handleListScheduleTemplates))
	mux.HandleFunc("POST /admin/schedule-templates", s.admin(s.handleCreateScheduleTemplate))
	mux.HandleFunc("PATCH /admin/schedule-templates/{id}", s.admin(s.handleUpdateScheduleTemplate))
	mux.HandleFunc("DELETE /admin/schedule-templates/{id}", s.admin(s.handleDeleteScheduleTemplate))
	mux.HandleFunc("POST /admin/schedule-templates/{id}/assign", s.admin(s.handleAssignScheduleTemplate))
	mux.HandleFunc("PATCH /admin/schedules/{id}", s.admin(s.handleUpdateSchedule))
	mux.HandleFunc("DELETE /admin/schedules/{id}", s.admin(s.handleDeleteSchedule))
	mux.HandleFunc("POST /admin/schedules/bulk", s.admin(s.handleBulkSchedules))
	mux.HandleFunc("POST /admin/schedules/import", s.admin(s.handleImportSchedules))
	mux.HandleFunc("GET /admin/departments", s.admin(s.handleListDepartments))
	mux.HandleFunc("POST /admin/departments", s.admin(s.handleCreateDepartment))
	mux.HandleFunc("POST /admin/departments/import", s.admin(s.handleImportDepartments))
	mux.HandleFunc("GET /admin/positions", s.admin(s.handleListPositions))
	mux.HandleFunc("POST /admin/positions", s.admin(s.handleCreatePosition))
	mux.HandleFunc("POST /admin/positions/import", s.admin(s.handleImportPositions))
	mux.HandleFunc("GET /admin/offices", s.admin(s.handleListOffices))
	mux.HandleFunc("POST /admin/offices", s.admin(s.handleCreateOffice))
	mux.HandleFunc("PATCH /admin/offices/{id}", s.admin(s.handleUpdateOffice))
	mux.HandleFunc("DELETE /admin/offices/{id}", s.admin(s.handleDeleteOffice))
	mux.HandleFunc("POST /admin/employees/import", s.admin(s.handleImportEmployees))
	mux.HandleFunc("POST /admin/users", s.admin(s.handleCreateUser))
	mux.HandleFunc("POST /admin/demo/seed", s.admin(s.handleSeedDemo))
	mux.HandleFunc("POST /admin/demo/reset", s.admin(s.handleResetDemo))
	mux.HandleFunc("POST /admin/demo/clear", s.admin(s.handleClearDemo))
	mux.HandleFunc("GET /admin/mobile-builds", s.admin(s.handleListMobileBuilds))
	mux.HandleFunc("POST /admin/mobile-builds", s.admin(s.handleUploadMobileBuild))
	mux.HandleFunc("DELETE /admin/mobile-builds/{id}", s.admin(s.handleDeleteMobileBuild))
	mux.HandleFunc("GET /mobile-builds", s.handlePublicMobileBuilds)
	mux.HandleFunc("GET /mobile-builds/{id}/download", s.handleDownloadMobileBuild)
	mux.HandleFunc("POST /admin/qr/static", s.admin(s.handleUploadStaticQR))
	mux.HandleFunc("POST /admin/qr/bulk", s.admin(s.handleBulkQR))
	mux.HandleFunc("POST /admin/qr/employees/bulk", s.admin(s.handleBulkEmployeeQR))
	mux.HandleFunc("POST /admin/qr/paper", s.admin(s.handleIssuePaperQR))
	mux.HandleFunc("POST /admin/qr/paper/{id}/revoke", s.admin(s.handleRevokePaperQR))
	mux.HandleFunc("GET /admin/qr/paper/{id}/pdf", s.admin(s.handlePaperQRPDF))
	mux.HandleFunc("POST /admin/qr/passes/pdf", s.admin(s.handlePaperPassesPDF))

	mux.HandleFunc("GET /notifications", s.authenticated(s.handleListNotifications))
	mux.HandleFunc("POST /notifications/{id}/read", s.authenticated(s.handleMarkNotificationRead))
	mux.HandleFunc("GET /delegations", s.authenticated(s.handleListDelegations))
	mux.HandleFunc("POST /delegations", s.authenticated(s.handleCreateDelegation))
	mux.HandleFunc("POST /delegations/{id}/approve", s.admin(s.handleApproveDelegation))
	mux.HandleFunc("POST /delegations/{id}/reject", s.admin(s.handleRejectDelegation))
	mux.HandleFunc("POST /delegations/{id}/revoke", s.authenticated(s.handleRevokeDelegation))

	mux.HandleFunc("POST /admin/offline-imports", s.admin(s.handleOfflineImport))
	mux.HandleFunc("GET /admin/oauth/clients", s.admin(s.handleListOAuthClients))
	mux.HandleFunc("POST /admin/oauth/clients", s.admin(s.handleCreateOAuthClient))
	mux.HandleFunc("PATCH /admin/oauth/clients/{id}", s.admin(s.handleUpdateOAuthClient))
	mux.HandleFunc("DELETE /admin/oauth/clients/{id}", s.admin(s.handleDeleteOAuthClient))
	mux.HandleFunc("POST /admin/oauth/clients/{id}/rotate", s.admin(s.handleRotateOAuthSecret))
	mux.HandleFunc("GET /admin/oauth/clients/{id}/whitelist", s.admin(s.handleListIPWhitelist))
	mux.HandleFunc("POST /admin/oauth/clients/{id}/whitelist", s.admin(s.handleAddIPWhitelist))

	mux.HandleFunc("POST /oauth/token", s.handleOAuthToken)
	mux.HandleFunc("GET /integration/1c/departments", s.oauth(s.handle1CDepartments))
	mux.HandleFunc("GET /integration/1c/positions", s.oauth(s.handle1CPositions))
	mux.HandleFunc("GET /integration/1c/employees", s.oauth(s.handle1CEmployees))
	mux.HandleFunc("GET /integration/1c/schedules", s.oauth(s.handle1CSchedules))
	mux.HandleFunc("GET /integration/1c/marks", s.oauth(s.handleExportMarks))
	mux.HandleFunc("GET /export/marks", s.oauth(s.handleExportMarks))
	mux.HandleFunc("POST /export/ack", s.oauth(s.handleExportAck))

	mux.HandleFunc("GET /metrics", metrics.handler)

	mux.HandleFunc("GET /reports/marks", s.reportsRole(s.handleReportMarks))
	mux.HandleFunc("GET /reports/shifts", s.reportsRole(s.handleReportShifts))
	mux.HandleFunc("GET /reports/summary", s.reportsRole(s.handleReportSummary))
	mux.HandleFunc("GET /reports/worked-time", s.reportsRole(s.handleReportWorkedTime))
	mux.HandleFunc("GET /reports/first-last", s.reportsRole(s.handleReportFirstLast))
	mux.HandleFunc("GET /reports/entries-exits", s.reportsRole(s.handleReportEntriesExits))
	mux.HandleFunc("GET /reports/daily", s.reportsRole(s.handleReportDaily))
	mux.HandleFunc("GET /reports/timesheet", s.reportsRole(s.handleReportTimesheet))

	return withCORS(s.cfg.CORSAllowedOrigins, withMetrics(mux))
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	pool, rdb := s.connections()
	if pool == nil || rdb == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": "not_configured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": "database"})
		return
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": "redis"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func withCORS(allowed []string, next http.Handler) http.Handler {
	allowAll := false
	origins := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		if o == "*" {
			allowAll = true
		}
		origins[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			_, ok := origins[origin]
			switch {
			case allowAll:
				w.Header().Set("Access-Control-Allow-Origin", "*")
			case ok:
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
