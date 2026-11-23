package auth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid username or password")

const issuer = "bookstore-api"

type User struct {
	Username string
	Password string
	Role     string
}

type storedUser struct {
	passwordHash []byte
	role         string
}

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type Service struct {
	secret    []byte
	users     map[string]storedUser
	dummyHash []byte
	ttl       time.Duration
}

func NewService(secret string, users []User, ttl time.Duration) (*Service, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT secret must be at least 32 bytes")
	}
	if ttl <= 0 {
		return nil, errors.New("token lifetime must be positive")
	}

	service := &Service{
		secret: []byte(secret),
		users:  make(map[string]storedUser, len(users)),
		ttl:    ttl,
	}
	dummyHash, err := bcrypt.GenerateFromPassword([]byte("invalid-bookstore-password"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("prepare password verifier: %w", err)
	}
	service.dummyHash = dummyHash

	for _, user := range users {
		if user.Username == "" || user.Password == "" {
			return nil, errors.New("configured usernames and passwords must not be empty")
		}
		if user.Role != "admin" && user.Role != "reader" {
			return nil, fmt.Errorf("unsupported role %q for user %q", user.Role, user.Username)
		}
		if _, exists := service.users[user.Username]; exists {
			return nil, fmt.Errorf("duplicate username %q", user.Username)
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password for user %q: %w", user.Username, err)
		}
		service.users[user.Username] = storedUser{passwordHash: passwordHash, role: user.Role}
	}
	if len(service.users) == 0 {
		return nil, errors.New("configure at least one user")
	}
	return service, nil
}

func (s *Service) Login(username, password string) (string, time.Time, error) {
	user, exists := s.users[username]
	hash := s.dummyHash
	if exists {
		hash = user.passwordHash
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte(password)); err != nil || !exists {
		return "", time.Time{}, ErrInvalidCredentials
	}

	expiresAt := time.Now().Add(s.ttl)
	claims := Claims{
		Role: user.role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

func (s *Service) RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowedRoles := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowedRoles[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			parts := strings.Fields(r.Header.Get("Authorization"))
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, http.StatusUnauthorized, "valid bearer token required")
				return
			}

			claims := &Claims{}
			token, err := jwt.ParseWithClaims(
				parts[1],
				claims,
				func(token *jwt.Token) (any, error) {
					if token.Method != jwt.SigningMethodHS256 {
						return nil, errors.New("unexpected signing method")
					}
					return s.secret, nil
				},
				jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
				jwt.WithIssuer(issuer),
			)
			if err != nil || token == nil || !token.Valid || claims.ExpiresAt == nil || claims.Subject == "" {
				w.Header().Set("WWW-Authenticate", "Bearer")
				writeError(w, http.StatusUnauthorized, "invalid or expired bearer token")
				return
			}

			roleAllowed := false
			for role := range allowedRoles {
				if subtle.ConstantTimeCompare([]byte(role), []byte(claims.Role)) == 1 {
					roleAllowed = true
					break
				}
			}
			if !roleAllowed {
				writeError(w, http.StatusForbidden, "insufficient role")
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type claimsContextKey struct{}

func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(*Claims)
	if !ok || claims == nil {
		return Claims{}, false
	}
	return *claims, true
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		slog.Error("write authentication error response failed", "error", err)
	}
}
