// Package service implements the business logic of the API.
package service

import (
	"context"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/tiktoday/kocflow-api/internal/config"
	"github.com/tiktoday/kocflow-api/internal/domain"
	"github.com/tiktoday/kocflow-api/internal/httpx"
	"github.com/tiktoday/kocflow-api/internal/repository"
)

const bcryptCost = 12

// AuthService registers and authenticates users.
type AuthService struct {
	users *repository.UserRepository
	cfg   config.Config
}

// NewAuthService builds an AuthService.
func NewAuthService(users *repository.UserRepository, cfg config.Config) *AuthService {
	return &AuthService{users: users, cfg: cfg}
}

// UserView is the public projection of a user.
type UserView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Session is the login/register response payload.
type Session struct {
	Token string   `json:"token"`
	User  UserView `json:"user"`
}

// Claims are the JWT claims embedded in access tokens.
type Claims struct {
	Role  string `json:"role"`
	Name  string `json:"name"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

func (s *AuthService) session(u *domain.User) (*Session, error) {
	now := time.Now()
	claims := Claims{
		Role: u.Role, Name: u.Name, Email: u.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID.Hex(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.JWTExpiresIn)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, err
	}
	return &Session{Token: token, User: UserView{ID: u.ID.Hex(), Name: u.Name, Email: u.Email, Role: u.Role}}, nil
}

// Register creates a member account.
func (s *AuthService) Register(ctx context.Context, name, email, password string) (*Session, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	existing, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, httpx.NewError(409, "Email đã được sử dụng")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, err
	}
	user := &domain.User{Name: strings.TrimSpace(name), Email: email, PasswordHash: string(hash), Role: "member"}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	return s.session(user)
}

// Login verifies credentials and issues a token.
func (s *AuthService) Login(ctx context.Context, email, password string) (*Session, error) {
	user, err := s.users.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return nil, err
	}
	if user == nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, httpx.NewError(401, "Email hoặc mật khẩu không đúng")
	}
	if err := s.users.TouchLogin(ctx, user); err != nil {
		return nil, err
	}
	return s.session(user)
}

// ParseToken validates a bearer token and returns its claims.
func (s *AuthService) ParseToken(token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// IssueTokenForTest signs a token for u without touching the database (used by HTTP tests).
func (s *AuthService) IssueTokenForTest(u *domain.User) (string, error) {
	sess, err := s.session(u)
	if err != nil {
		return "", err
	}
	return sess.Token, nil
}
