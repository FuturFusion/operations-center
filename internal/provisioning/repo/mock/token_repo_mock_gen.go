// Code generated by mockery; DO NOT EDIT.
// github.com/vektra/mockery
// template: matryer

package mock

import (
	"context"
	"sync"

	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/google/uuid"
)

// Ensure that TokenRepoMock does implement provisioning.TokenRepo.
// If this is not the case, regenerate this file with mockery.
var _ provisioning.TokenRepo = &TokenRepoMock{}

// TokenRepoMock is a mock implementation of provisioning.TokenRepo.
//
//	func TestSomethingThatUsesTokenRepo(t *testing.T) {
//
//		// make and configure a mocked provisioning.TokenRepo
//		mockedTokenRepo := &TokenRepoMock{
//			CreateFunc: func(ctx context.Context, token provisioning.Token) (int64, error) {
//				panic("mock out the Create method")
//			},
//			DeleteByUUIDFunc: func(ctx context.Context, id uuid.UUID) error {
//				panic("mock out the DeleteByUUID method")
//			},
//			GetAllFunc: func(ctx context.Context) (provisioning.Tokens, error) {
//				panic("mock out the GetAll method")
//			},
//			GetAllUUIDsFunc: func(ctx context.Context) ([]uuid.UUID, error) {
//				panic("mock out the GetAllUUIDs method")
//			},
//			GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
//				panic("mock out the GetByUUID method")
//			},
//			UpdateFunc: func(ctx context.Context, token provisioning.Token) error {
//				panic("mock out the Update method")
//			},
//		}
//
//		// use mockedTokenRepo in code that requires provisioning.TokenRepo
//		// and then make assertions.
//
//	}
type TokenRepoMock struct {
	// CreateFunc mocks the Create method.
	CreateFunc func(ctx context.Context, token provisioning.Token) (int64, error)

	// DeleteByUUIDFunc mocks the DeleteByUUID method.
	DeleteByUUIDFunc func(ctx context.Context, id uuid.UUID) error

	// GetAllFunc mocks the GetAll method.
	GetAllFunc func(ctx context.Context) (provisioning.Tokens, error)

	// GetAllUUIDsFunc mocks the GetAllUUIDs method.
	GetAllUUIDsFunc func(ctx context.Context) ([]uuid.UUID, error)

	// GetByUUIDFunc mocks the GetByUUID method.
	GetByUUIDFunc func(ctx context.Context, id uuid.UUID) (*provisioning.Token, error)

	// UpdateFunc mocks the Update method.
	UpdateFunc func(ctx context.Context, token provisioning.Token) error

	// calls tracks calls to the methods.
	calls struct {
		// Create holds details about calls to the Create method.
		Create []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Token is the token argument value.
			Token provisioning.Token
		}
		// DeleteByUUID holds details about calls to the DeleteByUUID method.
		DeleteByUUID []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// ID is the id argument value.
			ID uuid.UUID
		}
		// GetAll holds details about calls to the GetAll method.
		GetAll []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
		}
		// GetAllUUIDs holds details about calls to the GetAllUUIDs method.
		GetAllUUIDs []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
		}
		// GetByUUID holds details about calls to the GetByUUID method.
		GetByUUID []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// ID is the id argument value.
			ID uuid.UUID
		}
		// Update holds details about calls to the Update method.
		Update []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Token is the token argument value.
			Token provisioning.Token
		}
	}
	lockCreate       sync.RWMutex
	lockDeleteByUUID sync.RWMutex
	lockGetAll       sync.RWMutex
	lockGetAllUUIDs  sync.RWMutex
	lockGetByUUID    sync.RWMutex
	lockUpdate       sync.RWMutex
}

// Create calls CreateFunc.
func (mock *TokenRepoMock) Create(ctx context.Context, token provisioning.Token) (int64, error) {
	if mock.CreateFunc == nil {
		panic("TokenRepoMock.CreateFunc: method is nil but TokenRepo.Create was just called")
	}
	callInfo := struct {
		Ctx   context.Context
		Token provisioning.Token
	}{
		Ctx:   ctx,
		Token: token,
	}
	mock.lockCreate.Lock()
	mock.calls.Create = append(mock.calls.Create, callInfo)
	mock.lockCreate.Unlock()
	return mock.CreateFunc(ctx, token)
}

// CreateCalls gets all the calls that were made to Create.
// Check the length with:
//
//	len(mockedTokenRepo.CreateCalls())
func (mock *TokenRepoMock) CreateCalls() []struct {
	Ctx   context.Context
	Token provisioning.Token
} {
	var calls []struct {
		Ctx   context.Context
		Token provisioning.Token
	}
	mock.lockCreate.RLock()
	calls = mock.calls.Create
	mock.lockCreate.RUnlock()
	return calls
}

// DeleteByUUID calls DeleteByUUIDFunc.
func (mock *TokenRepoMock) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	if mock.DeleteByUUIDFunc == nil {
		panic("TokenRepoMock.DeleteByUUIDFunc: method is nil but TokenRepo.DeleteByUUID was just called")
	}
	callInfo := struct {
		Ctx context.Context
		ID  uuid.UUID
	}{
		Ctx: ctx,
		ID:  id,
	}
	mock.lockDeleteByUUID.Lock()
	mock.calls.DeleteByUUID = append(mock.calls.DeleteByUUID, callInfo)
	mock.lockDeleteByUUID.Unlock()
	return mock.DeleteByUUIDFunc(ctx, id)
}

// DeleteByUUIDCalls gets all the calls that were made to DeleteByUUID.
// Check the length with:
//
//	len(mockedTokenRepo.DeleteByUUIDCalls())
func (mock *TokenRepoMock) DeleteByUUIDCalls() []struct {
	Ctx context.Context
	ID  uuid.UUID
} {
	var calls []struct {
		Ctx context.Context
		ID  uuid.UUID
	}
	mock.lockDeleteByUUID.RLock()
	calls = mock.calls.DeleteByUUID
	mock.lockDeleteByUUID.RUnlock()
	return calls
}

// GetAll calls GetAllFunc.
func (mock *TokenRepoMock) GetAll(ctx context.Context) (provisioning.Tokens, error) {
	if mock.GetAllFunc == nil {
		panic("TokenRepoMock.GetAllFunc: method is nil but TokenRepo.GetAll was just called")
	}
	callInfo := struct {
		Ctx context.Context
	}{
		Ctx: ctx,
	}
	mock.lockGetAll.Lock()
	mock.calls.GetAll = append(mock.calls.GetAll, callInfo)
	mock.lockGetAll.Unlock()
	return mock.GetAllFunc(ctx)
}

// GetAllCalls gets all the calls that were made to GetAll.
// Check the length with:
//
//	len(mockedTokenRepo.GetAllCalls())
func (mock *TokenRepoMock) GetAllCalls() []struct {
	Ctx context.Context
} {
	var calls []struct {
		Ctx context.Context
	}
	mock.lockGetAll.RLock()
	calls = mock.calls.GetAll
	mock.lockGetAll.RUnlock()
	return calls
}

// GetAllUUIDs calls GetAllUUIDsFunc.
func (mock *TokenRepoMock) GetAllUUIDs(ctx context.Context) ([]uuid.UUID, error) {
	if mock.GetAllUUIDsFunc == nil {
		panic("TokenRepoMock.GetAllUUIDsFunc: method is nil but TokenRepo.GetAllUUIDs was just called")
	}
	callInfo := struct {
		Ctx context.Context
	}{
		Ctx: ctx,
	}
	mock.lockGetAllUUIDs.Lock()
	mock.calls.GetAllUUIDs = append(mock.calls.GetAllUUIDs, callInfo)
	mock.lockGetAllUUIDs.Unlock()
	return mock.GetAllUUIDsFunc(ctx)
}

// GetAllUUIDsCalls gets all the calls that were made to GetAllUUIDs.
// Check the length with:
//
//	len(mockedTokenRepo.GetAllUUIDsCalls())
func (mock *TokenRepoMock) GetAllUUIDsCalls() []struct {
	Ctx context.Context
} {
	var calls []struct {
		Ctx context.Context
	}
	mock.lockGetAllUUIDs.RLock()
	calls = mock.calls.GetAllUUIDs
	mock.lockGetAllUUIDs.RUnlock()
	return calls
}

// GetByUUID calls GetByUUIDFunc.
func (mock *TokenRepoMock) GetByUUID(ctx context.Context, id uuid.UUID) (*provisioning.Token, error) {
	if mock.GetByUUIDFunc == nil {
		panic("TokenRepoMock.GetByUUIDFunc: method is nil but TokenRepo.GetByUUID was just called")
	}
	callInfo := struct {
		Ctx context.Context
		ID  uuid.UUID
	}{
		Ctx: ctx,
		ID:  id,
	}
	mock.lockGetByUUID.Lock()
	mock.calls.GetByUUID = append(mock.calls.GetByUUID, callInfo)
	mock.lockGetByUUID.Unlock()
	return mock.GetByUUIDFunc(ctx, id)
}

// GetByUUIDCalls gets all the calls that were made to GetByUUID.
// Check the length with:
//
//	len(mockedTokenRepo.GetByUUIDCalls())
func (mock *TokenRepoMock) GetByUUIDCalls() []struct {
	Ctx context.Context
	ID  uuid.UUID
} {
	var calls []struct {
		Ctx context.Context
		ID  uuid.UUID
	}
	mock.lockGetByUUID.RLock()
	calls = mock.calls.GetByUUID
	mock.lockGetByUUID.RUnlock()
	return calls
}

// Update calls UpdateFunc.
func (mock *TokenRepoMock) Update(ctx context.Context, token provisioning.Token) error {
	if mock.UpdateFunc == nil {
		panic("TokenRepoMock.UpdateFunc: method is nil but TokenRepo.Update was just called")
	}
	callInfo := struct {
		Ctx   context.Context
		Token provisioning.Token
	}{
		Ctx:   ctx,
		Token: token,
	}
	mock.lockUpdate.Lock()
	mock.calls.Update = append(mock.calls.Update, callInfo)
	mock.lockUpdate.Unlock()
	return mock.UpdateFunc(ctx, token)
}

// UpdateCalls gets all the calls that were made to Update.
// Check the length with:
//
//	len(mockedTokenRepo.UpdateCalls())
func (mock *TokenRepoMock) UpdateCalls() []struct {
	Ctx   context.Context
	Token provisioning.Token
} {
	var calls []struct {
		Ctx   context.Context
		Token provisioning.Token
	}
	mock.lockUpdate.RLock()
	calls = mock.calls.Update
	mock.lockUpdate.RUnlock()
	return calls
}
