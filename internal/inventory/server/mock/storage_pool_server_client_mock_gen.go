// Code generated by mockery; DO NOT EDIT.
// github.com/vektra/mockery
// template: matryer

package mock

import (
	"context"
	"sync"

	"github.com/FuturFusion/operations-center/internal/inventory"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/lxc/incus/v6/shared/api"
)

// Ensure that StoragePoolServerClientMock does implement inventory.StoragePoolServerClient.
// If this is not the case, regenerate this file with mockery.
var _ inventory.StoragePoolServerClient = &StoragePoolServerClientMock{}

// StoragePoolServerClientMock is a mock implementation of inventory.StoragePoolServerClient.
//
//	func TestSomethingThatUsesStoragePoolServerClient(t *testing.T) {
//
//		// make and configure a mocked inventory.StoragePoolServerClient
//		mockedStoragePoolServerClient := &StoragePoolServerClientMock{
//			GetStoragePoolByNameFunc: func(ctx context.Context, cluster provisioning.Cluster, storagePoolName string) (api.StoragePool, error) {
//				panic("mock out the GetStoragePoolByName method")
//			},
//			GetStoragePoolsFunc: func(ctx context.Context, cluster provisioning.Cluster) ([]api.StoragePool, error) {
//				panic("mock out the GetStoragePools method")
//			},
//		}
//
//		// use mockedStoragePoolServerClient in code that requires inventory.StoragePoolServerClient
//		// and then make assertions.
//
//	}
type StoragePoolServerClientMock struct {
	// GetStoragePoolByNameFunc mocks the GetStoragePoolByName method.
	GetStoragePoolByNameFunc func(ctx context.Context, cluster provisioning.Cluster, storagePoolName string) (api.StoragePool, error)

	// GetStoragePoolsFunc mocks the GetStoragePools method.
	GetStoragePoolsFunc func(ctx context.Context, cluster provisioning.Cluster) ([]api.StoragePool, error)

	// calls tracks calls to the methods.
	calls struct {
		// GetStoragePoolByName holds details about calls to the GetStoragePoolByName method.
		GetStoragePoolByName []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Cluster is the cluster argument value.
			Cluster provisioning.Cluster
			// StoragePoolName is the storagePoolName argument value.
			StoragePoolName string
		}
		// GetStoragePools holds details about calls to the GetStoragePools method.
		GetStoragePools []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
			// Cluster is the cluster argument value.
			Cluster provisioning.Cluster
		}
	}
	lockGetStoragePoolByName sync.RWMutex
	lockGetStoragePools      sync.RWMutex
}

// GetStoragePoolByName calls GetStoragePoolByNameFunc.
func (mock *StoragePoolServerClientMock) GetStoragePoolByName(ctx context.Context, cluster provisioning.Cluster, storagePoolName string) (api.StoragePool, error) {
	if mock.GetStoragePoolByNameFunc == nil {
		panic("StoragePoolServerClientMock.GetStoragePoolByNameFunc: method is nil but StoragePoolServerClient.GetStoragePoolByName was just called")
	}
	callInfo := struct {
		Ctx             context.Context
		Cluster         provisioning.Cluster
		StoragePoolName string
	}{
		Ctx:             ctx,
		Cluster:         cluster,
		StoragePoolName: storagePoolName,
	}
	mock.lockGetStoragePoolByName.Lock()
	mock.calls.GetStoragePoolByName = append(mock.calls.GetStoragePoolByName, callInfo)
	mock.lockGetStoragePoolByName.Unlock()
	return mock.GetStoragePoolByNameFunc(ctx, cluster, storagePoolName)
}

// GetStoragePoolByNameCalls gets all the calls that were made to GetStoragePoolByName.
// Check the length with:
//
//	len(mockedStoragePoolServerClient.GetStoragePoolByNameCalls())
func (mock *StoragePoolServerClientMock) GetStoragePoolByNameCalls() []struct {
	Ctx             context.Context
	Cluster         provisioning.Cluster
	StoragePoolName string
} {
	var calls []struct {
		Ctx             context.Context
		Cluster         provisioning.Cluster
		StoragePoolName string
	}
	mock.lockGetStoragePoolByName.RLock()
	calls = mock.calls.GetStoragePoolByName
	mock.lockGetStoragePoolByName.RUnlock()
	return calls
}

// GetStoragePools calls GetStoragePoolsFunc.
func (mock *StoragePoolServerClientMock) GetStoragePools(ctx context.Context, cluster provisioning.Cluster) ([]api.StoragePool, error) {
	if mock.GetStoragePoolsFunc == nil {
		panic("StoragePoolServerClientMock.GetStoragePoolsFunc: method is nil but StoragePoolServerClient.GetStoragePools was just called")
	}
	callInfo := struct {
		Ctx     context.Context
		Cluster provisioning.Cluster
	}{
		Ctx:     ctx,
		Cluster: cluster,
	}
	mock.lockGetStoragePools.Lock()
	mock.calls.GetStoragePools = append(mock.calls.GetStoragePools, callInfo)
	mock.lockGetStoragePools.Unlock()
	return mock.GetStoragePoolsFunc(ctx, cluster)
}

// GetStoragePoolsCalls gets all the calls that were made to GetStoragePools.
// Check the length with:
//
//	len(mockedStoragePoolServerClient.GetStoragePoolsCalls())
func (mock *StoragePoolServerClientMock) GetStoragePoolsCalls() []struct {
	Ctx     context.Context
	Cluster provisioning.Cluster
} {
	var calls []struct {
		Ctx     context.Context
		Cluster provisioning.Cluster
	}
	mock.lockGetStoragePools.RLock()
	calls = mock.calls.GetStoragePools
	mock.lockGetStoragePools.RUnlock()
	return calls
}
