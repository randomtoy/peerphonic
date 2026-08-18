package peerphonic

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
)

func registerUserRoutes(mux *http.ServeMux, authenticator ports.Authenticator, users ports.UserManager) {
	mux.HandleFunc("GET /api/v1/session", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := requireAdministrator(writer, request, authenticator)
		if !ok {
			return
		}
		writeJSON(writer, http.StatusOK, newUserResponse(actor))
	})
	mux.HandleFunc("GET /api/v1/users", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := requireAdministrator(writer, request, authenticator)
		if !ok {
			return
		}
		items, err := users.Users(request.Context(), actor)
		if err != nil {
			writeUserError(writer, err)
			return
		}
		response := usersResponse{Users: make([]userResponse, 0, len(items))}
		for _, item := range items {
			response.Users = append(response.Users, newUserResponse(item))
		}
		writeJSON(writer, http.StatusOK, response)
	})
	mux.HandleFunc("POST /api/v1/users", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := requireAdministrator(writer, request, authenticator)
		if !ok {
			return
		}
		var payload struct {
			Username string          `json:"username"`
			Password string          `json:"password"`
			Role     domain.UserRole `json:"role"`
		}
		if err := decodeUserRequest(writer, request, &payload); err != nil {
			http.Error(writer, "invalid user request", http.StatusBadRequest)
			return
		}
		created, err := users.CreateUser(
			request.Context(), actor, payload.Username, payload.Password, payload.Role,
		)
		if err != nil {
			writeUserError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, newUserResponse(created))
	})
	mux.HandleFunc("PUT /api/v1/users/{username}/password", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := requireAdministrator(writer, request, authenticator)
		if !ok {
			return
		}
		var payload struct {
			Password string `json:"password"`
		}
		if err := decodeUserRequest(writer, request, &payload); err != nil {
			http.Error(writer, "invalid password request", http.StatusBadRequest)
			return
		}
		if err := users.UpdatePassword(
			request.Context(), actor, request.PathValue("username"), payload.Password,
		); err != nil {
			writeUserError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /api/v1/users/{username}", func(writer http.ResponseWriter, request *http.Request) {
		actor, ok := requireAdministrator(writer, request, authenticator)
		if !ok {
			return
		}
		if err := users.DeleteUser(request.Context(), actor, request.PathValue("username")); err != nil {
			writeUserError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
}

func decodeUserRequest(writer http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request contains trailing data")
	}
	return nil
}

func newUserResponse(user domain.User) userResponse {
	return userResponse{
		Username: user.Username, Role: user.Role,
		CreatedAt: user.CreatedAt.Format(time.RFC3339), UpdatedAt: user.UpdatedAt.Format(time.RFC3339),
	}
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeUserError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidUser):
		http.Error(writer, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ports.ErrAlreadyExists), errors.Is(err, ports.ErrLastAdministrator):
		http.Error(writer, err.Error(), http.StatusConflict)
	case errors.Is(err, ports.ErrNotFound):
		http.Error(writer, "user not found", http.StatusNotFound)
	case errors.Is(err, ports.ErrForbidden):
		http.Error(writer, "administrator access required", http.StatusForbidden)
	default:
		http.Error(writer, "manage users", http.StatusInternalServerError)
	}
}
