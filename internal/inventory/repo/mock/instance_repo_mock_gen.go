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

// Ensure that InstanceRepoMock does implement inventory.InstanceRepo.
// If this is not the case, regenerate this file with mockery.
var _ inventory.InstanceRepo = &InstanceRepoMock{}

// InstanceRepoMock is a mock implementation of inventory.InstanceRepo.
//
//	func TestSomethingThatUsesInstanceRepo(t *testing.T) {
//
//		// make and configure a mocked inventory.InstanceRepo
//		mockedInstanceRepo := &InstanceRepoMock{
//			CreateFunc: func(ctx context.Context, instance inventory.Instance) (inventory.Instance, error) {
//				panic("mock out the Create method")
//			},
//			DeleteByClusterNameFunc: func(ctx context.Context, cluster string) error {
//				panic("mock out the DeleteByClusterName method")
//			},
//			DeleteByUUIDFunc: func(ctx context.Context, id uuid.UUID) error {
//				panic("mock out the DeleteByUUID method")
//			},
//			GetAllUUIDsWithFilterFunc: func(ctx context.Context, filter inventory.InstanceFilter) ([]uuid.UUID, error) {
//				panic("mock out the GetAllUUIDsWithFilter method")
//			},
//			GetAllWithFilterFunc: func(ctx context.Context, filter inventory.InstanceFilter) (inventory.Instances, error) {
//				panic("mock out the GetAllWithFilter method")
//			},
//			GetByUUIDFunc: func(ctx context.Context, id uuid.UUID) (inventory.Instance, error) {
//				panic("mock out the GetByUUID method")
//			},
//			UpdateByUUIDFunc: func(ctx context.Context, instance inventory.Instance) (inventory.Instance, error) {
//				panic("mock out the UpdateByUUID method")
//			},
//		}
//
//		// use mockedInstanceRepo in code that requires inventory.InstanceRepo
//		// and then make assertions.
//
//	}
type InstanceRepoMock struct {
	// CreateFunc mocks the Create method.
	CreateFunc func(ctx context.Context, instance inventory.Instance) (inventory.Instance, error)

	// DeleteByClusterNameFunc mocks the DeleteByClusterName method.
	DeleteByClusterNameFunc func(ctx context.Context, cluster string) error

	// DeleteByUUIDFunc mocks the DeleteByUUID method.
	DeleteByUUIDFunc func(ctx context.Context, id uuid.UUID) error

	// GetAllUUIDsWithFilterFunc mocks the GetAllUUIDsWithFilter method.
	GetAllUUIDsWithFilterFunc func(ctx context.Context, filter inventory.InstanceFilter) ([]uuid.UUID, error)

	// GetAllWithFilterFunc mocks the GetAllWithFilter method.
	GetAllWithFilterFunc func(ctx context.Context, filter inventory.InstanceFilter) (inventory.Instances, error)

	// GetByUUIDFunc mocks the GetByUUID method.
	GetByUUIDFunc func(ctx context.Context, id uuid.UUID) (inventory.Instance, error)

	// UpdateByUUIDFunc mocks the UpdateByUUID method.
	UpdateByUUIDFunc func(ctx context.Context, instance inventory.Instance) (inventory.Instance, error)

	// calls tracks calls to the methods.
	calls struct {
		// Create holds details about calls to the Create method.
		Create []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Instance is the instance argument value.
			Instance inventory.Instance
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
			Filter inventory.InstanceFilter
		}
		// GetAllWithFilter holds details about calls to the GetAllWithFilter method.
		GetAllWithFilter []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Filter is the filter argument value.
			Filter inventory.InstanceFilter
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
			// Instance is the instance argument value.
			Instance inventory.Instance
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
func (mock *InstanceRepoMock) Create(ctx context.Context, instance inventory.Instance) (inventory.Instance, error) {
	if mock.CreateFunc == nil {
		panic("InstanceRepoMock.CreateFunc: method is nil but InstanceRepo.Create was just called")
	}
	callInfo := struct {
		Ctx      context.Context
		Instance inventory.Instance
	}{
		Ctx:      ctx,
		Instance: instance,
	}
	mock.lockCreate.Lock()
	mock.calls.Create = append(mock.calls.Create, callInfo)
	mock.lockCreate.Unlock()
	return mock.CreateFunc(ctx, instance)
}

// CreateCalls gets all the calls that were made to Create.
// Check the length with:
//
//	len(mockedInstanceRepo.CreateCalls())
func (mock *InstanceRepoMock) CreateCalls() []struct {
	Ctx      context.Context
	Instance inventory.Instance
} {
	var calls []struct {
		Ctx      context.Context
		Instance inventory.Instance
	}
	mock.lockCreate.RLock()
	calls = mock.calls.Create
	mock.lockCreate.RUnlock()
	return calls
}

// DeleteByClusterName calls DeleteByClusterNameFunc.
func (mock *InstanceRepoMock) DeleteByClusterName(ctx context.Context, cluster string) error {
	if mock.DeleteByClusterNameFunc == nil {
		panic("InstanceRepoMock.DeleteByClusterNameFunc: method is nil but InstanceRepo.DeleteByClusterName was just called")
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
//	len(mockedInstanceRepo.DeleteByClusterNameCalls())
func (mock *InstanceRepoMock) DeleteByClusterNameCalls() []struct {
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
func (mock *InstanceRepoMock) DeleteByUUID(ctx context.Context, id uuid.UUID) error {
	if mock.DeleteByUUIDFunc == nil {
		panic("InstanceRepoMock.DeleteByUUIDFunc: method is nil but InstanceRepo.DeleteByUUID was just called")
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
//	len(mockedInstanceRepo.DeleteByUUIDCalls())
func (mock *InstanceRepoMock) DeleteByUUIDCalls() []struct {
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
func (mock *InstanceRepoMock) GetAllUUIDsWithFilter(ctx context.Context, filter inventory.InstanceFilter) ([]uuid.UUID, error) {
	if mock.GetAllUUIDsWithFilterFunc == nil {
		panic("InstanceRepoMock.GetAllUUIDsWithFilterFunc: method is nil but InstanceRepo.GetAllUUIDsWithFilter was just called")
	}
	callInfo := struct {
		Ctx    context.Context
		Filter inventory.InstanceFilter
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
//	len(mockedInstanceRepo.GetAllUUIDsWithFilterCalls())
func (mock *InstanceRepoMock) GetAllUUIDsWithFilterCalls() []struct {
	Ctx    context.Context
	Filter inventory.InstanceFilter
} {
	var calls []struct {
		Ctx    context.Context
		Filter inventory.InstanceFilter
	}
	mock.lockGetAllUUIDsWithFilter.RLock()
	calls = mock.calls.GetAllUUIDsWithFilter
	mock.lockGetAllUUIDsWithFilter.RUnlock()
	return calls
}

// GetAllWithFilter calls GetAllWithFilterFunc.
func (mock *InstanceRepoMock) GetAllWithFilter(ctx context.Context, filter inventory.InstanceFilter) (inventory.Instances, error) {
	if mock.GetAllWithFilterFunc == nil {
		panic("InstanceRepoMock.GetAllWithFilterFunc: method is nil but InstanceRepo.GetAllWithFilter was just called")
	}
	callInfo := struct {
		Ctx    context.Context
		Filter inventory.InstanceFilter
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
//	len(mockedInstanceRepo.GetAllWithFilterCalls())
func (mock *InstanceRepoMock) GetAllWithFilterCalls() []struct {
	Ctx    context.Context
	Filter inventory.InstanceFilter
} {
	var calls []struct {
		Ctx    context.Context
		Filter inventory.InstanceFilter
	}
	mock.lockGetAllWithFilter.RLock()
	calls = mock.calls.GetAllWithFilter
	mock.lockGetAllWithFilter.RUnlock()
	return calls
}

// GetByUUID calls GetByUUIDFunc.
func (mock *InstanceRepoMock) GetByUUID(ctx context.Context, id uuid.UUID) (inventory.Instance, error) {
	if mock.GetByUUIDFunc == nil {
		panic("InstanceRepoMock.GetByUUIDFunc: method is nil but InstanceRepo.GetByUUID was just called")
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
//	len(mockedInstanceRepo.GetByUUIDCalls())
func (mock *InstanceRepoMock) GetByUUIDCalls() []struct {
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
func (mock *InstanceRepoMock) UpdateByUUID(ctx context.Context, instance inventory.Instance) (inventory.Instance, error) {
	if mock.UpdateByUUIDFunc == nil {
		panic("InstanceRepoMock.UpdateByUUIDFunc: method is nil but InstanceRepo.UpdateByUUID was just called")
	}
	callInfo := struct {
		Ctx      context.Context
		Instance inventory.Instance
	}{
		Ctx:      ctx,
		Instance: instance,
	}
	mock.lockUpdateByUUID.Lock()
	mock.calls.UpdateByUUID = append(mock.calls.UpdateByUUID, callInfo)
	mock.lockUpdateByUUID.Unlock()
	return mock.UpdateByUUIDFunc(ctx, instance)
}

// UpdateByUUIDCalls gets all the calls that were made to UpdateByUUID.
// Check the length with:
//
//	len(mockedInstanceRepo.UpdateByUUIDCalls())
func (mock *InstanceRepoMock) UpdateByUUIDCalls() []struct {
	Ctx      context.Context
	Instance inventory.Instance
} {
	var calls []struct {
		Ctx      context.Context
		Instance inventory.Instance
	}
	mock.lockUpdateByUUID.RLock()
	calls = mock.calls.UpdateByUUID
	mock.lockUpdateByUUID.RUnlock()
	return calls
}
