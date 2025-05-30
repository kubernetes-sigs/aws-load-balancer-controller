package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type LoaderError interface {
	Error() string
	GetRawError() error
	GetGatewayReason() gwv1.GatewayConditionReason
	GetRouteReason() gwv1.RouteConditionReason
	GetGatewayMessage() string
	GetRouteMessage() string
}

type loaderErrorImpl struct {
	underlyingErr            error // contains initial error message
	resolvedGatewayCondition gwv1.GatewayConditionReason
	resolvedRouteCondition   gwv1.RouteConditionReason
	resolvedGatewayMessage   *string
	resolvedRouteMessage     *string
}

func wrapError(underlyingErr error, gatewayCondition gwv1.GatewayConditionReason, routeCondition gwv1.RouteConditionReason, resolvedGatewayMessage *string, resolvedRouteMessage *string) LoaderError {
	e := &loaderErrorImpl{
		underlyingErr:            underlyingErr,
		resolvedGatewayCondition: gatewayCondition,
		resolvedRouteCondition:   routeCondition,
	}
	if resolvedGatewayMessage != nil {
		e.resolvedGatewayMessage = resolvedGatewayMessage
	}

	if resolvedRouteMessage != nil {
		e.resolvedRouteMessage = resolvedRouteMessage
	}
	return e
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

// GetGatewayMessage original error message for gateway status update can be overridden by pass in resolvedGatewayMessage
func (e *loaderErrorImpl) GetGatewayMessage() string {
	if e.resolvedGatewayMessage != nil {
		return *e.resolvedGatewayMessage
	}
	return e.underlyingErr.Error()
}

// GetRouteMessage original error message for route status update can be overridden by pass in resolvedRouteMessage
func (e *loaderErrorImpl) GetRouteMessage() string {
	if e.resolvedRouteMessage != nil {
		return *e.resolvedRouteMessage
	}
	return e.underlyingErr.Error()
}

func (e *loaderErrorImpl) Error() string {
	return e.underlyingErr.Error()
}
