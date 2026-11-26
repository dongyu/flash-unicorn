package security

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"unicorn/internal/domain"
)

type BcryptHasher struct {
	cost int
}

func NewBcryptHasher(cost int) *BcryptHasher {
	return &BcryptHasher{cost: cost}
}

var _ domain.PasswordHasher = (*BcryptHasher)(nil)

func (h *BcryptHasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash password: %w", err)
	}
	return string(hash), nil
}

func (h *BcryptHasher) Verify(password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return fmt.Errorf("bcrypt verify password: %w", domain.ErrUnauthorized)
	}
	return nil
}
