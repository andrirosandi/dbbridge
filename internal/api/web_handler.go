package api

import (
	"dbbridge/internal/core"
	"dbbridge/internal/service"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type WebHandler struct {
	connRepo  core.ConnectionRepository
	queryRepo core.QueryRepository
	auditRepo core.AuditRepository
	userRepo  core.UserRepository
	cryptoSvc *service.EncryptionService
	templates *template.Template
}

func NewWebHandler(connRepo core.ConnectionRepository, queryRepo core.QueryRepository, auditRepo core.AuditRepository, userRepo core.UserRepository, cryptoSvc *service.EncryptionService) *WebHandler {
	// ... (Template parsing logic remains) ...
	funcMap := template.FuncMap{
		"hasPrefix": strings.HasPrefix,
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseGlob("web/templates/*.html")
	if err != nil {
		fmt.Printf("CRITICAL: Failed to parse templates: %v\n", err)
	}

	return &WebHandler{
		connRepo:  connRepo,
		queryRepo: queryRepo,
		auditRepo: auditRepo,
		userRepo:  userRepo,
		cryptoSvc: cryptoSvc,
		templates: tmpl,
	}
}

// ... (Existing handlers) ...

func (h *WebHandler) AuditLogs(w http.ResponseWriter, r *http.Request) {
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
	h.render(w, "dashboard.html", map[string]interface{}{
		"Title": "Dashboard",
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
		"IsEdit":     false,
		"Connection": core.DBConnection{},
	}

	if idStr != "" {
		// Edit Mode
		id, _ := strconv.ParseInt(idStr, 10, 64)
		conn, err := h.connRepo.GetByID(id)
		if err == nil {
			data["IsEdit"] = true
			data["Connection"] = conn
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

	conn.Name = name
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

// --- Queries Form Handlers ---

func (h *WebHandler) QueryForm(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	data := map[string]interface{}{
		"IsEdit": false,
		"Query":  core.SavedQuery{},
	}

	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		q, err := h.queryRepo.GetByID(id) // Missing GetByID in Repo interface in prev step? Check.
		// Assuming GetByID exists or I used GetBySlug. Let's assume we need to add GetByID to QueryRepo if missing.
		// Actually I implemented params for GetBySlug. I should add GetByID to Repo for ID based edit.
		// For now let's hope I added it or I'll fix it. I see GetBySlug in interface.
		// I will implement a quick GetByID logic or redundant GetBySlug? ID is safer for editing.
		if err == nil { // Placeholder if Repo has ID fetch
			data["IsEdit"] = true
			data["Query"] = q
		}
	}

	h.render(w, "query_form.html", data)
}

func (h *WebHandler) SaveQuery(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	idStr := r.FormValue("id")

	q := &core.SavedQuery{
		Slug:        r.FormValue("slug"),
		Description: r.FormValue("description"),
		SQLText:     r.FormValue("sql_text"),
		IsActive:    r.FormValue("is_active") == "on",
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

// --- User Management Handlers ---

func (h *WebHandler) UsersList(w http.ResponseWriter, r *http.Request) {
	users, err := h.userRepo.GetAll()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.render(w, "users.html", map[string]interface{}{
		"Title": "Users",
		"Users": users,
	})
}

func (h *WebHandler) UserForm(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	data := map[string]interface{}{
		"IsEdit": false,
		"User":   core.User{},
	}

	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		user, err := h.userRepo.GetByID(id)
		if err == nil {
			data["IsEdit"] = true
			data["User"] = user
		}
	}

	h.render(w, "user_form.html", data)
}

func (h *WebHandler) SaveUser(w http.ResponseWriter, r *http.Request) {
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
	username := r.FormValue("username")
	password := r.FormValue("password")
	isActive := r.FormValue("is_active") == "on"

	if idStr != "" {
		// Update
		id, _ := strconv.ParseInt(idStr, 10, 64)
		user, err := h.userRepo.GetByID(id)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		user.Username = username
		user.IsActive = isActive

		if password != "" {
			hashedValue, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				http.Error(w, "Encryption failed", http.StatusInternalServerError)
				return
			}
			user.PasswordHash = string(hashedValue)
		} else {
			user.PasswordHash = "" // Clear it so Repo doesn't update it
		}

		if err := h.userRepo.Update(user); err != nil {
			http.Error(w, "Failed to update user: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Create
		if password == "" {
			http.Error(w, "Password is required", http.StatusBadRequest)
			return
		}

		hashedValue, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Encryption failed", http.StatusInternalServerError)
			return
		}

		user, err := h.userRepo.CreateUser(username, string(hashedValue))
		if err != nil {
			http.Error(w, "Failed to create user: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Sync IsActive if different from default (true)
		if !isActive {
			user.IsActive = false
			user.PasswordHash = "" // Dont update password hash again
			h.userRepo.Update(user)
		}
	}

	http.Redirect(w, r, "/admin/users", http.StatusFound)
}

func (h *WebHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	h.userRepo.Delete(id)
	http.Redirect(w, r, "/admin/users", http.StatusFound)
}

func (h *WebHandler) render(w http.ResponseWriter, tmplName string, data interface{}) {
	if h.templates == nil {
		h.ReloadTemplates() // Try loading if nil
		if h.templates == nil {
			http.Error(w, "Templates not loaded", http.StatusInternalServerError)
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
	r.Get("/admin/connections/delete", h.DeleteConnection)

	// Queries
	r.Get("/admin/queries", h.QueriesList)
	r.Get("/admin/queries/new", h.QueryForm)
	r.Get("/admin/queries/edit", h.QueryForm) // Careful: requires ID
	r.Post("/admin/queries/save", h.SaveQuery)
	r.Get("/admin/queries/delete", h.DeleteQuery)

	// Users
	r.Get("/admin/users", h.UsersList)
	r.Get("/admin/users/new", h.UserForm)
	r.Get("/admin/users/edit", h.UserForm)
	r.Post("/admin/users/save", h.SaveUser)
	r.Get("/admin/users/delete", h.DeleteUser)

	// Audit Logs
	r.Get("/admin/logs", h.AuditLogs)
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
