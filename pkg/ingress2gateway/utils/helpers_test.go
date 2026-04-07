package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceNameDeterministic(t *testing.T) {
	a := GetHTTPRouteName("default", "my-app")
	b := GetHTTPRouteName("default", "my-app")
	assert.Equal(t, a, b, "same inputs must produce same name")
}

func TestResourceNameUniqueness(t *testing.T) {
	// Different resource types for same ingress must not collide
	gw := GetGatewayName("ns", "app")
	rt := GetHTTPRouteName("ns", "app")
	lb := GetLBConfigName("ns", "app")
	dr := GetDefaultHTTPRouteName("ns", "app")
	tg := GetTGConfigName("ns", "app")

	names := []string{gw, rt, lb, dr, tg}
	seen := make(map[string]bool)
	for _, n := range names {
		assert.False(t, seen[n], "collision detected: %s", n)
		seen[n] = true
	}
}

func TestResourceNameDifferentNamespaces(t *testing.T) {
	a := GetHTTPRouteName("ns1", "app")
	b := GetHTTPRouteName("ns2", "app")
	assert.NotEqual(t, a, b, "different namespaces must produce different names")
}

func TestResourceNameNoCollisionAcrossTypes(t *testing.T) {
	// An ingress named "foo-default" should not collide with the default route for ingress "foo"
	routeForFooDefault := GetHTTPRouteName("ns", "foo-default")
	defaultRouteForFoo := GetDefaultHTTPRouteName("ns", "foo")
	assert.NotEqual(t, routeForFooDefault, defaultRouteForFoo)
}

func TestGetRedirectHTTPRouteNameUnique(t *testing.T) {
	redirect := GetRedirectHTTPRouteName("ns", "app")
	route := GetHTTPRouteName("ns", "app")
	defaultRoute := GetDefaultHTTPRouteName("ns", "app")
	assert.NotEqual(t, redirect, route, "redirect route name must differ from primary route")
	assert.NotEqual(t, redirect, defaultRoute, "redirect route name must differ from default route")
}
