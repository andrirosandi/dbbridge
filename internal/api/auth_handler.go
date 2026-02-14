package api

import (
	"dbbridge/internal/service"
	"html/template"
	"net/http"

	"github.com/gorilla/sessions"
)

type AuthHandler struct {
	authSvc   *service.AuthService
	store     *sessions.CookieStore
	templates *template.Template
}

func NewAuthHandler(authSvc *service.AuthService, sessionKey string, templates *template.Template) *AuthHandler {
	// Use DBBRIDGE_KEY for session encryption too
	store := sessions.NewCookieStore([]byte(sessionKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   false, // Set to true if HTTPS
	}

	return &AuthHandler{
		authSvc:   authSvc,
		store:     store,
		templates: templates,
	}
}

func (h *AuthHandler) SetupPage(w http.ResponseWriter, r *http.Request) {
	hasUsers, _ := h.authSvc.HasUsers()
	if hasUsers {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	h.render(w, "setup.html", nil)
}

func (h *AuthHandler) DoSetup(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	err := h.authSvc.SetupAdmin(username, password)
	if err != nil {
		h.render(w, "setup.html", map[string]interface{}{"Error": err.Error()})
		return
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}

func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	hasUsers, _ := h.authSvc.HasUsers()
	if !hasUsers {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	h.render(w, "login.html", nil)
}

func (h *AuthHandler) DoLogin(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := h.authSvc.Authenticate(username, password)
	if err != nil {
		h.render(w, "login.html", map[string]interface{}{"Error": "Invalid username or password"})
		return
	}

	// Set Session
	session, _ := h.store.Get(r, "dbbridge-session")
	session.Values["user_id"] = user.ID
	session.Values["username"] = user.Username
	session.Save(r, w)

	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	session, _ := h.store.Get(r, "dbbridge-session")
	session.Options.MaxAge = -1
	session.Save(r, w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

// Middleware to protect admin routes
func (h *AuthHandler) AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bypass for static files
		// if strings.HasPrefix(r.URL.Path, "/static") {
		// 	next.ServeHTTP(w, r)
		// 	return
		// }

		session, _ := h.store.Get(r, "dbbridge-session")
		if auth, ok := session.Values["user_id"].(int64); !ok || auth == 0 {
			// Check if setup is needed
			hasUsers, _ := h.authSvc.HasUsers()
			if !hasUsers && r.URL.Path != "/setup" {
				http.Redirect(w, r, "/setup", http.StatusFound)
				return
			}

			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (h *AuthHandler) render(w http.ResponseWriter, tmplName string, data interface{}) {
	if h.templates == nil {
		http.Error(w, "AuthTemplates not loaded", http.StatusInternalServerError)
		return
	}

	// Create a new template executor for these standalone pages if not part of main layout
	err := h.templates.ExecuteTemplate(w, tmplName, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
