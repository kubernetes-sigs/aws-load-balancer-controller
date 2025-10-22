package shared_utils

import (
	"context"
	"k8s.io/apimachinery/pkg/util/cache"
)

type MockTargetGroupARNMapper struct {
	ARN   string
	Error error
}

func (m *MockTargetGroupARNMapper) GetArnByName(_ context.Context, _ string) (string, error) {
	return m.ARN, m.Error
}

func (m *MockTargetGroupARNMapper) GetCache() *cache.Expiring {
	return nil
}

var _ TargetGroupARNMapper = &MockTargetGroupARNMapper{}
