package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type LoaderError interface {
	Error() string
	GetRawError() error
	GetGatewayReason() gwv1.GatewayConditionReason
	GetRouteReason() gwv1.RouteConditionReason
}

type loaderErrorImpl struct {
	resolvedGatewayCondition gwv1.GatewayConditionReason
	resolvedRouteCondition   gwv1.RouteConditionReason
	underlyingErr            error
}

func wrapError(underlyingErr error, gatewayCondition gwv1.GatewayConditionReason, routeCondition gwv1.RouteConditionReason) LoaderError {
	return &loaderErrorImpl{
		underlyingErr:            underlyingErr,
		resolvedGatewayCondition: gatewayCondition,
		resolvedRouteCondition:   routeCondition,
	}
}

func (e *loaderErrorImpl) GetRawError() error {
	return e.underlyingErr
}

func (e *loaderErrorImpl) GetGatewayReason() gwv1.GatewayConditionReason {
	return e.resolvedGatewayCondition
}

func (e *loaderErrorImpl) GetRouteReason() gwv1.RouteConditionReason {
	return e.resolvedRouteCondition
}

func (e *loaderErrorImpl) Error() string {
	return e.underlyingErr.Error()
}
