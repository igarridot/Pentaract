package service

import (
	"context"
	"time"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
	"github.com/Dominux/Pentaract/internal/password"
	"github.com/Dominux/Pentaract/internal/repository"
)

type AuthService struct {
	usersRepo *repository.UsersRepo
	secretKey string
	expireIn  time.Duration
}

func NewAuthService(usersRepo *repository.UsersRepo, secretKey string, expireInSecs int) *AuthService {
	return &AuthService{
		usersRepo: usersRepo,
		secretKey: secretKey,
		expireIn:  time.Duration(expireInSecs) * time.Second,
	}
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func (s *AuthService) Login(ctx context.Context, email, pass string) (*LoginResponse, error) {
	user, err := s.usersRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, domain.ErrNotAuthenticated()
	}

	if err := password.Verify(pass, user.PasswordHash); err != nil {
		return nil, domain.ErrNotAuthenticated()
	}

	token, err := appjwt.Generate(appjwt.AuthUser{
		ID:    user.ID,
		Email: user.Email,
	}, s.expireIn, s.secretKey)
	if err != nil {
		return nil, domain.ErrInternal("failed to generate token")
	}

	return &LoginResponse{
		AccessToken: token,
		TokenType:   "bearer",
		ExpiresIn:   int(s.expireIn.Seconds()),
	}, nil
}
