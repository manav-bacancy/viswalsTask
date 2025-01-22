package mockredis

import (
	"context"
	"github.com/stretchr/testify/mock"
	"github.com/viswals_task/core/models"
)

type MockRedis struct {
	mock.Mock
}

func (m *MockRedis) Get(ctx context.Context, key string) (*models.UserDetails, error) {
	args := m.Called(ctx, key)
	return args.Get(0).(*models.UserDetails), args.Error(1)
}

func (m *MockRedis) Set(ctx context.Context, key string, data *models.UserDetails) error {
	args := m.Called(ctx, key, data)
	return args.Error(0)
}

func (m *MockRedis) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}
