package api

import (
	"context"
	"database/sql"
	"dbbridge/internal/config"
	"dbbridge/internal/core"
	"dbbridge/internal/logger"
	"dbbridge/internal/service"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

type WebHandler struct {
	connRepo     core.ConnectionRepository
	queryRepo    core.QueryRepository
	auditRepo    core.AuditRepository
	userRepo     core.UserRepository
	cryptoSvc    *service.EncryptionService
	templates    *template.Template
	apiKeyRepo   core.ApiKeyRepository
	authSvc      *service.AuthService
	config       *config.Config
	executor     *service.QueryExecutor
	sessionStore *sessions.CookieStore
}

func NewWebHandler(connRepo core.ConnectionRepository, queryRepo core.QueryRepository, auditRepo core.AuditRepository, userRepo core.UserRepository, apiKeyRepo core.ApiKeyRepository, authSvc *service.AuthService, cryptoSvc *service.EncryptionService, cfg *config.Config) *WebHandler {
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
	}

	tmpl, err := template.New("layout.html").Funcs(funcMap).ParseGlob("web/templates/*.html")
	if err != nil {
		logger.Error.Fatalf("Failed to parse templates: %v", err)
	}

	executor := service.NewQueryExecutor(connRepo, queryRepo, auditRepo, cryptoSvc)

	// Create session store with the same key as AuthHandler
	store := sessions.NewCookieStore([]byte(cfg.DbBridgeKey))

	return &WebHandler{
		connRepo:     connRepo,
		queryRepo:    queryRepo,
		auditRepo:    auditRepo,
		userRepo:     userRepo,
		cryptoSvc:    cryptoSvc,
		apiKeyRepo:   apiKeyRepo,
		authSvc:      authSvc,
		config:       cfg,
		templates:    tmpl,
		executor:     executor,
		sessionStore: store,
	}
}

// ... (Existing handlers) ...

func (h *WebHandler) HandleAuditLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := h.auditRepo.GetRecent(100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.render(w, "audit_logs.html", map[string]interface{}{
		"Title": "Audit Logs",
		"Logs":  logs,
	})
}

// ReloadTemplates helper for development (optional)
func (h *WebHandler) ReloadTemplates() {
	funcMap := template.FuncMap{
		"hasPrefix": strings.HasPrefix,
	}
	var err error
	h.templates, err = template.New("").Funcs(funcMap).ParseGlob("web/templates/*.html")
	if err != nil {
		fmt.Printf("CRITICAL: Failed to reload templates: %v\n", err)
	}
}

func (h *WebHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	// 1. Logs
	logs, err := h.auditRepo.GetRecent(5)
	if err != nil {
		logger.Error.Printf("Dashboard: Failed to get logs: %v", err)
	}

	// 2. Connections
	conns, err := h.connRepo.GetAll()
	activeConns := 0
	if err == nil {
		for _, c := range conns {
			if c.IsActive {
				activeConns++
			}
		}
	}

	// 3. Queries
	queries, err := h.queryRepo.GetAll()
	activeQueries := 0
	if err == nil {
		for _, q := range queries {
			if q.IsActive {
				activeQueries++
			}
		}
	}

	// 4. Users
	users, err := h.userRepo.GetAll()
	userCount := 0
	if err == nil {
		userCount = len(users)
	}

	h.render(w, "dashboard.html", map[string]interface{}{
		"Title":         "Dashboard",
		"Logs":          logs,
		"TotalConns":    len(conns),
		"ActiveConns":   activeConns,
		"TotalQueries":  len(queries),
		"ActiveQueries": activeQueries,
		"TotalUsers":    userCount,
	})
}

func (h *WebHandler) ConnectionsList(w http.ResponseWriter, r *http.Request) {
	conns, err := h.connRepo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.render(w, "connections.html", map[string]interface{}{
		"Title":       "Connections",
		"Connections": conns,
	})
}

func (h *WebHandler) QueriesList(w http.ResponseWriter, r *http.Request) {
	queries, err := h.queryRepo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.render(w, "queries.html", map[string]interface{}{
		"Title":   "Queries",
		"Queries": queries,
	})
}

// --- Connections Form Handlers ---

func (h *WebHandler) ConnectionForm(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	data := map[string]interface{}{
		"IsEdit":           false,
		"Connection":       core.DBConnection{},
		"SupportedDrivers": h.config.SupportedDrivers,
	}

	if idStr != "" {
		// Edit Mode
		id, _ := strconv.ParseInt(idStr, 10, 64)
		conn, err := h.connRepo.GetByID(id)
		if err == nil {
			data["IsEdit"] = true
			data["Connection"] = conn

			// Decrypt for display
			decrypted, err := h.cryptoSvc.Decrypt(conn.ConnectionStringEnc)
			if err == nil {
				data["ConnectionStringDec"] = decrypted
			}
		}
	}

	h.render(w, "connection_form.html", data)
}

func (h *WebHandler) SaveConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	idStr := r.FormValue("id")
	name := r.FormValue("name")
	driver := r.FormValue("driver")
	rawConnStr := r.FormValue("connection_string")
	isActive := r.FormValue("is_active") == "on"

	var conn *core.DBConnection
	if idStr != "" {
		// Update
		id, _ := strconv.ParseInt(idStr, 10, 64)
		conn, _ = h.connRepo.GetByID(id)
	} else {
		// New
		conn = &core.DBConnection{}
	}

	conn.Name = core.Slugify(name)
	conn.Driver = driver
	conn.IsActive = isActive

	// Only update password if provided or new
	if rawConnStr != "" {
		encStr, err := h.cryptoSvc.Encrypt(rawConnStr)
		if err != nil {
			http.Error(w, "Encryption failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		conn.ConnectionStringEnc = encStr
	}

	if conn.ID != 0 {
		h.connRepo.Update(conn)
	} else {
		h.connRepo.Create(conn)
	}

	http.Redirect(w, r, "/admin/connections", http.StatusFound)
}

func (h *WebHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	h.connRepo.Delete(id)
	http.Redirect(w, r, "/admin/connections", http.StatusFound)
}

// TestConnection attempts to ping the database with provided details
func (h *WebHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	driver := r.FormValue("driver")
	connStr := r.FormValue("connection_string")

	if driver == "" || connStr == "" {
		http.Error(w, "Driver and Connection String are required", http.StatusBadRequest)
		return
	}

	// Try to connect
	// Note: You might need to import the driver packages if they aren't already imported in main.go
	// Since main.go imports modernc.org/sqlite, and likely others should be imported there using _
	db, err := sql.Open(driver, connStr)
	if err != nil {
		http.Error(w, "Failed to open connection: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		http.Error(w, "Connection failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Connection successful!"))
}

// RunQuery executes a raw SQL query against a specific connection (for testing)
func (h *WebHandler) RunQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var params map[string]interface{}
	var connID int64
	var queryID int64
	var sqlText string
	var err error

	// Check content type to handle JSON or Form
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var req struct {
			ConnectionID int64                  `json:"connection_id"`
			QueryID      int64                  `json:"query_id"`
			SQLText      string                 `json:"sql_text"`
			Params       map[string]interface{} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON: " + err.Error()})
			return
		}
		connID = req.ConnectionID
		queryID = req.QueryID
		sqlText = req.SQLText
		params = req.Params // Can be nil
	} else {
		// Fallback to Form (existing behavior)
		connIDStr := r.FormValue("connection_id")
		queryIDStr := r.FormValue("query_id") // Optional
		sqlText = r.FormValue("sql_text")
		if connIDStr == "" || sqlText == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Connection ID and SQL Text are required"})
			return
		}
		connID, err = strconv.ParseInt(connIDStr, 10, 64)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid Connection ID"})
			return
		}
		if queryIDStr != "" {
			queryID, _ = strconv.ParseInt(queryIDStr, 10, 64)
		}
		// Form doesn't easily support map params without convention.
		// For now, keep params empty for Form.
		params = make(map[string]interface{})
	}

	if params == nil {
		params = make(map[string]interface{})
	}

	result, err := h.executor.ExecuteSQL(r.Context(), connID, sqlText, params, queryID)
	if err != nil {
		// Return JSON error to be friendly to frontend fetch
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "%s"}`, strings.ReplaceAll(err.Error(), "\"", "\\\""))
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Manual JSON marshaling to avoid importing "encoding/json" if not already there?
	// Actually "encoding/json" is standard. Let's use it.
	// I need to check imports. "encoding/json" is NOT in imports yet (only "database/sql", etc).
	// I'll add the import or use a simple string builder if I want to avoid re-imports logic complexity.
	// But adding import is cleaner.
	// Wait, I can't easily add import with replace_file_content if it's far away.
	// I'll use a hacky string build or just assume I can add import in another step.
	// Let's use the tool to add import first? No, too slow.
	// I will use fmt.Sprintf for now, or just json.Marshal if I trust I can add the import.
	// Let's add the import now in a separate step to be safe.

	// ... marshaling
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, "Failed to encode result", http.StatusInternalServerError)
	}
}

// --- Queries Form Handlers ---

func (h *WebHandler) QueryForm(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")

	// Fetch all connections for the checkbox list
	conns, err := h.connRepo.GetAll()
	if err != nil {
		http.Error(w, "Failed to load connections: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"IsEdit":      false,
		"Query":       core.SavedQuery{},
		"Connections": conns,
	}

	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		q, err := h.queryRepo.GetByID(id)
		if err == nil {
			data["IsEdit"] = true
			data["Query"] = q
		}
	}

	h.render(w, "query_form.html", data)
}

func (h *WebHandler) SaveQuery(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	idStr := r.FormValue("id")

	// Parse selected connections
	var connIDs []int64
	if r.PostForm.Has("connection_ids") {
		for _, idVal := range r.PostForm["connection_ids"] {
			id, _ := strconv.ParseInt(idVal, 10, 64)
			connIDs = append(connIDs, id)
		}
	}

	q := &core.SavedQuery{
		Slug:                 core.Slugify(r.FormValue("slug")),
		Description:          r.FormValue("description"),
		SQLText:              r.FormValue("sql_text"),
		IsActive:             r.FormValue("is_active") == "on",
		AllowedConnectionIDs: connIDs,
	}

	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		q.ID = id
		// For update we need to preserve things or just overwrite.
		// Repo Update usually takes full object.
		h.queryRepo.Update(q)
	} else {
		h.queryRepo.Create(q)
	}

	http.Redirect(w, r, "/admin/queries", http.StatusFound)
}

func (h *WebHandler) DeleteQuery(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	h.queryRepo.Delete(id)
	http.Redirect(w, r, "/admin/queries", http.StatusFound)
}

// --- My Profile Handlers ---

func (h *WebHandler) HandleProfile(w http.ResponseWriter, r *http.Request) {
	session, _ := h.sessionStore.Get(r, "dbbridge-session")
	userID, _ := session.Values["user_id"].(int64)
	username, _ := session.Values["username"].(string)

	// Check for flash messages
	successMsg := ""
	errorMsg := ""
	if msg, ok := session.Values["flash_success"].(string); ok && msg != "" {
		successMsg = msg
		delete(session.Values, "flash_success")
		session.Save(r, w)
	}
	if msg, ok := session.Values["flash_error"].(string); ok && msg != "" {
		errorMsg = msg
		delete(session.Values, "flash_error")
		session.Save(r, w)
	}

	h.render(w, "profile.html", map[string]interface{}{
		"Title":    "My Profile",
		"UserID":   userID,
		"Username": username,
		"Success":  successMsg,
		"Error":    errorMsg,
	})
}

func (h *WebHandler) HandleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	session, _ := h.sessionStore.Get(r, "dbbridge-session")
	userID, _ := session.Values["user_id"].(int64)

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate
	if newPassword == "" {
		session.Values["flash_error"] = "New password is required."
		session.Save(r, w)
		http.Redirect(w, r, "/admin/profile", http.StatusFound)
		return
	}
	if newPassword != confirmPassword {
		session.Values["flash_error"] = "New passwords do not match."
		session.Save(r, w)
		http.Redirect(w, r, "/admin/profile", http.StatusFound)
		return
	}

	// Verify current password
	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		session.Values["flash_error"] = "User not found."
		session.Save(r, w)
		http.Redirect(w, r, "/admin/profile", http.StatusFound)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		session.Values["flash_error"] = "Current password is incorrect."
		session.Save(r, w)
		http.Redirect(w, r, "/admin/profile", http.StatusFound)
		return
	}

	// Hash new password
	hashedValue, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		session.Values["flash_error"] = "Failed to update password."
		session.Save(r, w)
		http.Redirect(w, r, "/admin/profile", http.StatusFound)
		return
	}

	user.PasswordHash = string(hashedValue)
	if err := h.userRepo.Update(user); err != nil {
		session.Values["flash_error"] = "Failed to save password: " + err.Error()
		session.Save(r, w)
		http.Redirect(w, r, "/admin/profile", http.StatusFound)
		return
	}

	session.Values["flash_success"] = "Password updated successfully!"
	session.Save(r, w)
	http.Redirect(w, r, "/admin/profile", http.StatusFound)
}

// API Keys Management

func (h *WebHandler) HandleListApiKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.apiKeyRepo.List()
	if err != nil {
		h.render(w, "api_keys.html", map[string]interface{}{"Error": err.Error()})
		return
	}

	data := map[string]interface{}{
		"Title": "API Keys",
		"Keys":  keys,
	}
	h.render(w, "api_keys.html", data)
}

func (h *WebHandler) HandleCreateApiKey(w http.ResponseWriter, r *http.Request) {
	userID := int64(1) // Default to admin for now
	description := r.FormValue("description")

	key, apiKey, err := h.authSvc.GenerateApiKey(userID, description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	keys, _ := h.apiKeyRepo.List()

	data := map[string]interface{}{
		"Title":   "API Keys",
		"Keys":    keys,
		"NewKey":  key,
		"NewID":   apiKey.ID,
		"NewDesc": apiKey.Description,
	}
	h.render(w, "api_keys.html", data)
}

func (h *WebHandler) HandleRevokeApiKey(w http.ResponseWriter, r *http.Request) {
	idStr := r.FormValue("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	if err := h.apiKeyRepo.Revoke(int64(id)); err != nil {
		logger.Error.Printf("Failed to revoke key: %v", err)
	}
	http.Redirect(w, r, "/admin/api-keys", http.StatusFound)
}

func (h *WebHandler) render(w http.ResponseWriter, tmplName string, data interface{}) {
	if h.templates == nil {
		h.ReloadTemplates() // Try loading if nil
		if h.templates == nil {
			logger.Error.Println("WebHandler: Templates are nil after reload attempt")
			http.Error(w, "WebTemplates not loaded", http.StatusInternalServerError)
			return
		}
	}

	// Execute layout which should yield the specific template
	// Assuming layout.html defines {{block "content" .}}
	err := h.templates.ExecuteTemplate(w, "layout.html", map[string]interface{}{
		"Page": tmplName, // To identify active page
		"Data": data,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// GetTemplates returns the parsed templates (useful for sharing with AuthHandler)
func (h *WebHandler) GetTemplates() *template.Template {
	return h.templates
}

// Setup Routes for Web
func (h *WebHandler) RegisterRoutes(r chi.Router) {
	// Redirect root to admin
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/setup", http.StatusFound)
	})

	r.Get("/admin", h.Dashboard)

	// Connections
	r.Get("/admin/connections", h.ConnectionsList)
	r.Get("/admin/connections/new", h.ConnectionForm)
	r.Get("/admin/connections/edit", h.ConnectionForm)
	r.Post("/admin/connections/save", h.SaveConnection)
	r.Post("/admin/connections/test", h.TestConnection)
	r.Get("/admin/connections/delete", h.DeleteConnection)

	// Queries
	r.Get("/admin/queries", h.QueriesList)
	r.Get("/admin/queries/new", h.QueryForm)
	r.Get("/admin/queries/edit", h.QueryForm) // Careful: requires ID
	r.Post("/admin/queries/save", h.SaveQuery)
	r.Post("/admin/queries/run", h.RunQuery) // Test Run
	r.Get("/admin/queries/delete", h.DeleteQuery)

	// Profile
	r.Get("/admin/profile", h.HandleProfile)
	r.Post("/admin/profile", h.HandleUpdatePassword)

	r.Get("/admin/api-keys", h.HandleListApiKeys)
	r.Post("/admin/api-keys/create", h.HandleCreateApiKey)
	r.Post("/admin/api-keys/revoke", h.HandleRevokeApiKey)

	// Audit Logs
	r.Get("/admin/logs", h.HandleAuditLogs)
}

func (h *WebHandler) RegisterStatic(r chi.Router) {
	// Static files
	workDir := "."
	filesDir := http.Dir(filepath.Join(workDir, "web/static"))
	FileServer(r, "/static", filesDir)
}

// Simple file server helper for Chi
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
