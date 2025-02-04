package operations

import "github.com/google/uuid"

func WithRandomUUID(randomUUID func() (uuid.UUID, error)) TokenServiceOption {
	return func(s *tokenService) {
		s.randomUUID = randomUUID
	}
}
