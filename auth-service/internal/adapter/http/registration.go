package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"ecommerce.local/auth-service/internal/domain"
	"ecommerce.local/auth-service/internal/port"
)

type RegistrationHandler struct{ registration port.CustomerRegistration }

func NewRegistrationHandler(registration port.CustomerRegistration) *RegistrationHandler {
	return &RegistrationHandler{registration: registration}
}

type registerRequest struct {
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type customerResponse struct {
	UserID   string `json:"user_id"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

type apiError struct {
	Code    string `json:"error_code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type envelope struct {
	Data   any        `json:"data"`
	Errors []apiError `json:"errors"`
}

func (h *RegistrationHandler) Register(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		respondError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json", "")
		return
	}
	var request *registerRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request == nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request", "")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request", "")
		return
	}
	user, err := h.registration.Register(r.Context(), port.RegisterCustomerCommand{FullName: request.FullName, Email: request.Email, Password: request.Password})
	if err != nil {
		var validationError *domain.ValidationError
		switch {
		case errors.As(err, &validationError):
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationError.Message, validationError.Field)
		case errors.Is(err, domain.ErrEmailAlreadyRegistered):
			respondError(w, http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "email already registered", "email")
		default:
			slog.Error("registration failed", "error", err, "service", "auth-service")
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", "")
		}
		return
	}
	respondJSON(w, http.StatusCreated, envelope{Data: customerResponse{UserID: user.ID, FullName: user.FullName, Email: user.Email, Role: user.Role}, Errors: []apiError{}})
}

func respondError(w http.ResponseWriter, status int, code, message, field string) {
	respondJSON(w, status, envelope{Data: nil, Errors: []apiError{{Code: code, Message: message, Field: field}}})
}
func respondJSON(w http.ResponseWriter, status int, value envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type signInRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (h *RegistrationHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		respondError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json", "")
		return
	}
	var request *signInRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request == nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request", "")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON request", "")
		return
	}
	token, err := h.registration.SignIn(r.Context(), port.SignInCustomerCommand{Email: request.Email, Password: request.Password})
	if err != nil {
		var validationError *domain.ValidationError
		switch {
		case errors.As(err, &validationError):
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", validationError.Message, validationError.Field)
		case errors.Is(err, domain.ErrInvalidCredentials):
			respondError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password.", "")
		default:
			slog.Error("sign in failed", "error", err, "service", "auth-service")
			respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", "")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	respondJSON(w, http.StatusOK, envelope{
		Data:   accessTokenResponse{AccessToken: token.AccessToken, TokenType: token.TokenType, ExpiresIn: token.ExpiresIn},
		Errors: []apiError{},
	})
}
