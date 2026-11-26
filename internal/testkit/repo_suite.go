package testkit

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"unicorn/internal/domain"
)

type RepoHarness struct {
	TxManager        domain.TxManager
	NoteRepo         domain.NoteRepository
	UserRepo         domain.UserRepository
	RefreshTokenRepo domain.RefreshTokenRepository
	Enqueuer         domain.JobEnqueuer
	DeleteUser       func(userID uuid.UUID) error
	Cleanup          func()
}

type RepoSuite struct {
	suite.Suite
	NewHarness func() RepoHarness
	harness    RepoHarness
}

func (s *RepoSuite) SetupTest() {
	s.harness = s.NewHarness()
}

func (s *RepoSuite) TearDownTest() {
	if s.harness.Cleanup != nil {
		s.harness.Cleanup()
	}
}
