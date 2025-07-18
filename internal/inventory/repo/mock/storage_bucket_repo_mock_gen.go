// Code generated by mockery; DO NOT EDIT.
// github.com/vektra/mockery
// template: matryer

package mock

import (
	"context"
	"sync"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/google/uuid"
)

// Ensure that StorageBucketRepoMock does implement inventory.StorageBucketRepo.
// If this is not the case, regenerate this file with mockery.
var _ inventory.StorageBucketRepo = &StorageBucketRepoMock{}

// StorageBucketRepoMock is a mock implementation of inventory.StorageBucketRepo.
//
//	func TestSomethingThatUsesStorageBucketRepo(t *testing.T) {
//
//		// make and configure a mocked inventory.StorageBucketRepo
//		mockedStorageBucketRepo := &StorageBucketRepoMock{
//			CreateFunc: func(ctx context.Context, storageBucket inventory.StorageBucket) (inventory.StorageBucket, error) {
//				panic("mock out the Create method")
//			},
//			DeleteByClusterNameFunc: func(ctx context.Context, cluster string) error {
//				panic("mock out the DeleteByClusterName method")
//			},
//			DeleteByUUIDFunc: func(ctx context.Context, id uuid.UUID) error {
//				panic("mock out the DeleteByUUID method")
//			},
//			GetAllUUIDsWithFilterFunc: func(ctx context.Context, filter inventory.StorageBucketFilter) ([]uuid.UUID, error) {
//				panic("mock out the GetAllUUIDsWithFilter method")
//			},
//			GetAllWithFilterFunc: func(ctx context.Context, filter inventory.StorageBucketFilter) (inventory.StorageBuckets, error) {
//				panic("mock out the GetAllWithFilter method")
//			},
//			GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (inventory.StorageBucket, error) {
//				panic("mock out the GetByUUID method")
//			},
//			UpdateByUUIDFunc: func(ctx context.Context, storageBucket inventory.StorageBucket) (inventory.StorageBucket, error) {
//				panic("mock out the UpdateByUUID method")
//			},
//		}
//
//		// use mockedStorageBucketRepo in code that requires inventory.StorageBucketRepo
//		// and then make assertions.
//
//	}
type StorageBucketRepoMock struct {
	// CreateFunc mocks the Create method.
	CreateFunc func(ctx context.Context, storageBucket inventory.StorageBucket) (inventory.StorageBucket, error)

	// DeleteByClusterNameFunc mocks the DeleteByClusterName method.
	DeleteByClusterNameFunc func(ctx context.Context, cluster string) error

	// DeleteByUUIDFunc mocks the DeleteByUUID method.
	DeleteByUUIDFunc func(ctx context.Context, id uuid.UUID) error

	// GetAllUUIDsWithFilterFunc mocks the GetAllUUIDsWithFilter method.
	GetAllUUIDsWithFilterFunc func(ctx context.Context, filter inventory.StorageBucketFilter) ([]uuid.UUID, error)

	// GetAllWithFilterFunc mocks the GetAllWithFilter method.
	GetAllWithFilterFunc func(ctx context.Context, filter inventory.StorageBucketFilter) (inventory.StorageBuckets, error)

	// GetByUUIDFunc mocks the GetByUUID method.
	GetByUUIDFunc func(ctx context.Context, id uuid.UUID) (inventory.StorageBucket, error)

	// UpdateByUUIDFunc mocks the UpdateByUUID method.
	UpdateByUUIDFunc func(ctx context.Context, storageBucket inventory.StorageBucket) (inventory.StorageBucket, error)

	// calls tracks calls to the methods.
	calls struct {
		// Create holds details about calls to the Create method.
		Create []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// StorageBucket is the storageBucket argument value.
			StorageBucket inventory.StorageBucket
		}
		// DeleteByClusterName holds details about calls to the DeleteByClusterName method.
		DeleteByClusterName []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Cluster is the cluster argument value.
			Cluster string
		}
		// DeleteByUUID holds details about calls to the DeleteByUUID method.
		DeleteByUUID []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// ID is the id argument value.
			ID uuid.UUID
		}
		// GetAllUUIDsWithFilter holds details about calls to the GetAllUUIDsWithFilter method.
		GetAllUUIDsWithFilter []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Filter is the filter argument value.
			Filter inventory.StorageBucketFilter
		}
		// GetAllWithFilter holds details about calls to the GetAllWithFilter method.
		GetAllWithFilter []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Filter is the filter argument value.
			Filter inventory.StorageBucketFilter
		}
		// GetByUUID holds details about calls to the GetByUUID method.
		GetByUUID []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// ID is the id argument value.
			ID uuid.UUID
		}
		// UpdateByUUID holds details about calls to the UpdateByUUID method.
		UpdateByUUID []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// StorageBucket is the storageBucket argument value.
			StorageBucket inventory.StorageBucket
		}
	}
	lockCreate                sync.RWMutex
	lockDeleteByClusterName   sync.RWMutex
	lockDeleteByUUID          sync.RWMutex
	lockGetAllUUIDsWithFilter sync.RWMutex
	lockGetAllWithFilter      sync.RWMutex
	lockGetByUUID             sync.RWMutex
	lockUpdateByUUID          sync.RWMutex
}

// Create calls CreateFunc.
func (mock *StorageBucketRepoMock) Create(ctx context.Context, storageBucket inventory.StorageBucket) (inventory.StorageBucket, error) {
	if mock.CreateFunc == nil {
		panic("StorageBucketRepoMock.CreateFunc: method is nil but StorageBucketRepo.Create was just called")
	}
	callInfo := struct {
		Ctx           context.Context
		StorageBucket inventory.StorageBucket
	}{
		Ctx:           ctx,
		StorageBucket: storageBucket,
	}
	mock.lockCreate.Lock()
	mock.calls.Create = append(mock.calls.Create, callInfo)
	mock.lockCreate.Unlock()
	return mock.CreateFunc(ctx, storageBucket)
}

// CreateCalls gets all the calls that were made to Create.
// Check the length with:
//
//	len(mockedStorageBucketRepo.CreateCalls())
func (mock *StorageBucketRepoMock) CreateCalls() []struct {
	Ctx           context.Context
	StorageBucket inventory.StorageBucket
} {
	var calls []struct {
		Ctx           context.Context
		StorageBucket inventory.StorageBucket
	}
	mock.lockCreate.RLock()
	calls = mock.calls.Create
	mock.lockCreate.RUnlock()
	return calls
}

// DeleteByClusterName calls DeleteByClusterNameFunc.
func (mock *StorageBucketRepoMock) DeleteByClusterName(ctx context.Context, cluster string) error {
	if mock.DeleteByClusterNameFunc == nil {
		panic("StorageBucketRepoMock.DeleteByClusterNameFunc: method is nil but StorageBucketRepo.DeleteByClusterName was just called")
	}
	callInfo := struct {
		Ctx     context.Context
		Cluster string
	}{
		Ctx:     ctx,
		Cluster: cluster,
	}
	mock.lockDeleteByClusterName.Lock()
	mock.calls.DeleteByClusterName = append(mock.calls.DeleteByClusterName, callInfo)
	mock.lockDeleteByClusterName.Unlock()
	return mock.DeleteByClusterNameFunc(ctx, cluster)
}

// DeleteByClusterNameCalls gets all the calls that were made to DeleteByClusterName.
// Check the length with:
//
//	len(mockedStorageBucketRepo.DeleteByClusterNameCalls())
func (mock *StorageBucketRepoMock) DeleteByClusterNameCalls() []struct {
	Ctx     context.Context
	Cluster string
} {
	var calls []struct {
		Ctx     context.Context
		Cluster string
	}
	mock.lockDeleteByClusterName.RLock()
	calls = mock.calls.DeleteByClusterName
	mock.lockDeleteByClusterName.RUnlock()
	return calls
}

// DeleteByUUID calls DeleteByUUIDFunc.
func (mock *StorageBucketRepoMock) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	if mock.DeleteByUUIDFunc == nil {
		panic("StorageBucketRepoMock.DeleteByUUIDFunc: method is nil but StorageBucketRepo.DeleteByUUID was just called")
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
//	len(mockedStorageBucketRepo.DeleteByUUIDCalls())
func (mock *StorageBucketRepoMock) DeleteByUUIDCalls() []struct {
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

// GetAllUUIDsWithFilter calls GetAllUUIDsWithFilterFunc.
func (mock *StorageBucketRepoMock) GetAllUUIDsWithFilter(ctx context.Context, filter inventory.StorageBucketFilter) ([]uuid.UUID, error) {
	if mock.GetAllUUIDsWithFilterFunc == nil {
		panic("StorageBucketRepoMock.GetAllUUIDsWithFilterFunc: method is nil but StorageBucketRepo.GetAllUUIDsWithFilter was just called")
	}
	callInfo := struct {
		Ctx    context.Context
		Filter inventory.StorageBucketFilter
	}{
		Ctx:    ctx,
		Filter: filter,
	}
	mock.lockGetAllUUIDsWithFilter.Lock()
	mock.calls.GetAllUUIDsWithFilter = append(mock.calls.GetAllUUIDsWithFilter, callInfo)
	mock.lockGetAllUUIDsWithFilter.Unlock()
	return mock.GetAllUUIDsWithFilterFunc(ctx, filter)
}

// GetAllUUIDsWithFilterCalls gets all the calls that were made to GetAllUUIDsWithFilter.
// Check the length with:
//
//	len(mockedStorageBucketRepo.GetAllUUIDsWithFilterCalls())
func (mock *StorageBucketRepoMock) GetAllUUIDsWithFilterCalls() []struct {
	Ctx    context.Context
	Filter inventory.StorageBucketFilter
} {
	var calls []struct {
		Ctx    context.Context
		Filter inventory.StorageBucketFilter
	}
	mock.lockGetAllUUIDsWithFilter.RLock()
	calls = mock.calls.GetAllUUIDsWithFilter
	mock.lockGetAllUUIDsWithFilter.RUnlock()
	return calls
}

// GetAllWithFilter calls GetAllWithFilterFunc.
func (mock *StorageBucketRepoMock) GetAllWithFilter(ctx context.Context, filter inventory.StorageBucketFilter) (inventory.StorageBuckets, error) {
	if mock.GetAllWithFilterFunc == nil {
		panic("StorageBucketRepoMock.GetAllWithFilterFunc: method is nil but StorageBucketRepo.GetAllWithFilter was just called")
	}
	callInfo := struct {
		Ctx    context.Context
		Filter inventory.StorageBucketFilter
	}{
		Ctx:    ctx,
		Filter: filter,
	}
	mock.lockGetAllWithFilter.Lock()
	mock.calls.GetAllWithFilter = append(mock.calls.GetAllWithFilter, callInfo)
	mock.lockGetAllWithFilter.Unlock()
	return mock.GetAllWithFilterFunc(ctx, filter)
}

// GetAllWithFilterCalls gets all the calls that were made to GetAllWithFilter.
// Check the length with:
//
//	len(mockedStorageBucketRepo.GetAllWithFilterCalls())
func (mock *StorageBucketRepoMock) GetAllWithFilterCalls() []struct {
	Ctx    context.Context
	Filter inventory.StorageBucketFilter
} {
	var calls []struct {
		Ctx    context.Context
		Filter inventory.StorageBucketFilter
	}
	mock.lockGetAllWithFilter.RLock()
	calls = mock.calls.GetAllWithFilter
	mock.lockGetAllWithFilter.RUnlock()
	return calls
}

// GetByUUID calls GetByUUIDFunc.
func (mock *StorageBucketRepoMock) GetByUUID(ctx context.Context, id uuid.UUID) (inventory.StorageBucket, error) {
	if mock.GetByUUIDFunc == nil {
		panic("StorageBucketRepoMock.GetByUUIDFunc: method is nil but StorageBucketRepo.GetByUUID was just called")
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
//	len(mockedStorageBucketRepo.GetByUUIDCalls())
func (mock *StorageBucketRepoMock) GetByUUIDCalls() []struct {
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

// UpdateByUUID calls UpdateByUUIDFunc.
func (mock *StorageBucketRepoMock) UpdateByUUID(ctx context.Context, storageBucket inventory.StorageBucket) (inventory.StorageBucket, error) {
	if mock.UpdateByUUIDFunc == nil {
		panic("StorageBucketRepoMock.UpdateByUUIDFunc: method is nil but StorageBucketRepo.UpdateByUUID was just called")
	}
	callInfo := struct {
		Ctx           context.Context
		StorageBucket inventory.StorageBucket
	}{
		Ctx:           ctx,
		StorageBucket: storageBucket,
	}
	mock.lockUpdateByUUID.Lock()
	mock.calls.UpdateByUUID = append(mock.calls.UpdateByUUID, callInfo)
	mock.lockUpdateByUUID.Unlock()
	return mock.UpdateByUUIDFunc(ctx, storageBucket)
}

// UpdateByUUIDCalls gets all the calls that were made to UpdateByUUID.
// Check the length with:
//
//	len(mockedStorageBucketRepo.UpdateByUUIDCalls())
func (mock *StorageBucketRepoMock) UpdateByUUIDCalls() []struct {
	Ctx           context.Context
	StorageBucket inventory.StorageBucket
} {
	var calls []struct {
		Ctx           context.Context
		StorageBucket inventory.StorageBucket
	}
	mock.lockUpdateByUUID.RLock()
	calls = mock.calls.UpdateByUUID
	mock.lockUpdateByUUID.RUnlock()
	return calls
}
