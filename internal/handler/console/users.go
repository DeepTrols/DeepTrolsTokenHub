package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	userRepo "github.com/deeptrols/api/internal/repository/user"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type userListResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	UserType    string    `json:"user_type"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// HandleListUsers returns users with pagination, optionally filtered by
// ?user_type=personal|enterprise (admin only).
func HandleListUsers(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		filter := userRepo.ListFilter{}
		if raw := r.URL.Query().Get("user_type"); raw != "" {
			if raw != string(domain.UserTypePersonal) && raw != string(domain.UserTypeEnterprise) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user_type"})
				return
			}
			filter.UserType = domain.UserType(raw)
		}
		limit, offset := parsePagination(r)
		users, err := a.Users.List(r.Context(), filter, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list users"})
			return
		}
		total, err := a.Users.Count(r.Context(), filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to count users"})
			return
		}

		response := make([]userListResponse, 0, len(users))
		for _, u := range users {
			response = append(response, userListResponse{
				ID: u.ID.String(), Email: u.Email, DisplayName: u.DisplayName,
				Role: u.Role, UserType: string(u.UserType), Status: string(u.Status),
				CreatedAt: u.CreatedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": response, "total": total})
	}
}

// HandleUpdateUserStatus updates a user's status (active/banned/deleted). Admin only.
func HandleUpdateUserStatus(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		userID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		switch domain.UserStatus(req.Status) {
		case domain.UserStatusActive, domain.UserStatusBanned, domain.UserStatusDeleted:
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid status"})
			return
		}

		// Prevent banning/deleting yourself.
		actorID, _ := jwtutil.UserIDFromContext(r.Context())
		if userID == actorID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot modify your own status"})
			return
		}

		if err := a.Users.UpdateStatus(r.Context(), userID, domain.UserStatus(req.Status)); err != nil {
			if errors.Is(err, userRepo.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update user status"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// HandleUpdateUserRole updates a user's role (user/admin). Admin only.
func HandleUpdateUserRole(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		userID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}
		var req struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Role != "user" && req.Role != "admin" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid role"})
			return
		}
		if err := a.Users.UpdateRole(r.Context(), userID, req.Role); err != nil {
			if errors.Is(err, userRepo.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update role"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// HandleUpdateProfile updates the authenticated user's display name.
func HandleUpdateProfile(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		var req struct {
			DisplayName string `json:"display_name"`
			Phone       string `json:"phone,omitempty"`
			AvatarURL   string `json:"avatar_url,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.DisplayName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name is required"})
			return
		}
		if err := a.Users.UpdateProfile(r.Context(), userID, req.DisplayName, req.Phone, req.AvatarURL); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update profile"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// HandleChangePassword changes the authenticated user's password after
// verifying the current one.
func HandleChangePassword(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		var req struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if len(req.NewPassword) < 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "New password must be at least 8 characters"})
			return
		}

		u, err := a.Users.FindByID(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.CurrentPassword)) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Current password is incorrect"})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to hash password"})
			return
		}
		if err := a.Users.UpdatePassword(r.Context(), userID, string(hash)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update password"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// HandleCreateUser creates a new user. Admin only.
func HandleCreateUser(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		var req struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
			Role        string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Email == "" || req.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password are required"})
			return
		}
		if len(req.Password) < 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Password must be at least 8 characters"})
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}
		if req.DisplayName == "" {
			req.DisplayName = req.Email
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to hash password"})
			return
		}

		now := time.Now().UTC()
		u := &domain.User{
			ID:           uuid.New(),
			Email:        req.Email,
			PasswordHash: string(hash),
			DisplayName:  req.DisplayName,
			Role:         req.Role,
			Status:       domain.UserStatusActive,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := a.Users.Create(r.Context(), u); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user: " + err.Error()})
			return
		}

		// Also create a wallet for the new user.
		a.Pool.Exec(r.Context(),
			`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
			 VALUES ($1, $2, '0', '0', 'CNY', 0, $3, $3)
			 ON CONFLICT DO NOTHING`,
			uuid.New(), u.ID, now,
		)

		writeJSON(w, http.StatusCreated, map[string]string{"id": u.ID.String(), "email": u.Email})
	}
}

// HandleDeleteUser permanently deletes a user. Admin only.
func HandleDeleteUser(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		userID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		// Prevent deleting yourself.
		actorID, _ := jwtutil.UserIDFromContext(r.Context())
		if userID == actorID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot delete yourself"})
			return
		}

		if err := a.Users.UpdateStatus(r.Context(), userID, domain.UserStatusDeleted); err != nil {
			if errors.Is(err, userRepo.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete user"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}
