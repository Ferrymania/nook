package service

import "github.com/Ferrymania/nook/internal/repository"

func IsTransientDatabaseLock(err error) bool {
	return repository.IsTransientDatabaseError(err)
}
