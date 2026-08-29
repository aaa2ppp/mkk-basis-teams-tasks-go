package sign

import (
	"context"
	"errors"
	"fmt"
	"unicode"

	"aaa2ppp/teams-tasks/internal/db"
	"aaa2ppp/teams-tasks/internal/model"

	"golang.org/x/crypto/bcrypt"
)

type Storage interface {
	WithTx(tx db.DBTX) Storage
	GetByEmail(ctx context.Context, email string) (model.User, error)
	GetByID(ctx context.Context, userID model.UserID) (model.User, error)
	GetRoles(ctx context.Context, userID model.UserID) (map[string]model.Role, error)
	Create(ctx context.Context, user model.User) (model.UserID, error)
}

type Transactor interface {
	InTx(context.Context, func(context.Context, db.DBTX) error) error
}

type TokenGenerator interface {
	GenerateToken(user model.User) (string, error)
}

type service struct {
	storage    Storage
	transactor Transactor
	token      TokenGenerator
}

var _ Service = &service{}

func NewService(storage Storage, transactor Transactor, token TokenGenerator) *service {
	return &service{
		storage:    storage,
		transactor: transactor,
		token:      token,
	}
}

func (s *service) Login(ctx context.Context, req LoginReq) (LoginResp, error) {
	var zero LoginResp

	user, err := s.storage.GetByEmail(ctx, req.Email)
	if err != nil {
		return zero, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return zero, errors.New("invalid password")
	}
	user.PasswordHash = ""

	roles, err := s.storage.GetRoles(ctx, user.ID)
	if err != nil {
		return zero, err
	}
	user.Roles = roles

	token, err := s.token.GenerateToken(user)
	if err != nil {
		return zero, err
	}

	return LoginResp{
		User:  user,
		Token: token,
	}, nil
}

func (s *service) Register(ctx context.Context, req RegisterReq) (LoginResp, error) {
	var zero LoginResp

	if err := validatePassword(req.Password); err != nil {
		return zero, fmt.Errorf("%w: %v", model.ErrValidation, err)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return zero, fmt.Errorf("failed to hash password: %w", err)
	}

	var user model.User
	if err := s.transactor.InTx(ctx, func(ctx context.Context, tx db.DBTX) (err error) {
		storage := s.storage.WithTx(tx)
		userID, err := storage.Create(ctx, model.User{
			Email:        req.Email,
			Name:         req.Name,
			PasswordHash: string(hashed),
		})
		if err != nil {
			return err
		}
		user, err = storage.GetByID(ctx, userID)
		return
	}); err != nil {
		return zero, err
	}
	user.PasswordHash = ""

	token, err := s.token.GenerateToken(user)
	if err != nil {
		return zero, err
	}

	return LoginResp{
		User:  user,
		Token: token,
	}, nil
}

func validatePassword(password string) error {
	count := 0
	hasDigit := false
	hasUpper := false
	hasLower := false
	hasOther := false
	for _, ch := range password {
		count++
		switch {
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		default:
			hasOther = true
		}
	}
	var errs []error
	if count < 8 {
		errs = append(errs, errors.New("password must be at least 8 characters long"))
	}
	if !hasDigit {
		errs = append(errs, errors.New("password must contain at least one digit"))
	}
	if !hasUpper {
		errs = append(errs, errors.New("password must contain at least one uppercase letter"))
	}
	if !hasLower {
		errs = append(errs, errors.New("password must contain at least one lowercase letter"))
	}
	if !hasOther {
		errs = append(errs, errors.New("password must contain at least one non-alphanumeric character"))
	}
	return errors.Join(errs...)
}
