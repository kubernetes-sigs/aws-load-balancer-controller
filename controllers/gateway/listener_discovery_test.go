package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDiscoverListeners(t *testing.T) {
	tests := []struct {
		name     string
		gateway  *gwv1.Gateway
		expected *DiscoveredListeners
	}{
		{
			name: "single listener",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{
							Name: "http",
							Port: 80,
						},
					},
				},
			},
			expected: &DiscoveredListeners{
				All: []DiscoveredListener{
					{
						Listener: gwv1.Listener{Name: "http", Port: 80},
						Index:    0,
						Port:     80,
						Name:     "http",
					},
				},
				ByPort:     map[int32]DiscoveredListener{80: {Listener: gwv1.Listener{Name: "http", Port: 80}, Index: 0, Port: 80, Name: "http"}},
				ByName:     map[gwv1.SectionName]DiscoveredListener{"http": {Listener: gwv1.Listener{Name: "http", Port: 80}, Index: 0, Port: 80, Name: "http"}},
				PortToName: map[int32]gwv1.SectionName{80: "http"},
			},
		},
		{
			name: "multiple listeners",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{
						{Name: "http", Port: 80},
						{Name: "https", Port: 443},
						{Name: "tcp", Port: 8080},
					},
				},
			},
		},
		{
			name: "empty listeners",
			gateway: &gwv1.Gateway{
				Spec: gwv1.GatewaySpec{
					Listeners: []gwv1.Listener{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discovered := DiscoverListeners(tt.gateway)

			// Test basic structure
			assert.Equal(t, len(tt.gateway.Spec.Listeners), len(discovered.All))
			assert.Equal(t, len(tt.gateway.Spec.Listeners), len(discovered.ByPort))
			assert.Equal(t, len(tt.gateway.Spec.Listeners), len(discovered.ByName))
			assert.Equal(t, len(tt.gateway.Spec.Listeners), len(discovered.PortToName))

			// Test specific case for single listener
			if tt.name == "single listener" {
				dl, exists := discovered.GetByPort(80)
				assert.True(t, exists)
				assert.Equal(t, gwv1.SectionName("http"), dl.Name)
				assert.Equal(t, int32(80), dl.Port)
				assert.Equal(t, 0, dl.Index)

				dl2, exists2 := discovered.GetByName("http")
				assert.True(t, exists2)
				assert.Equal(t, int32(80), dl2.Port)

				port, exists3 := discovered.GetPortByName("http")
				assert.True(t, exists3)
				assert.Equal(t, int32(80), port)

				name, exists4 := discovered.GetNameByPort(80)
				assert.True(t, exists4)
				assert.Equal(t, gwv1.SectionName("http"), name)
			}

			// Test multiple listeners case
			if tt.name == "multiple listeners" {
				ports := discovered.Ports()
				assert.Contains(t, ports, int32(80))
				assert.Contains(t, ports, int32(443))
				assert.Contains(t, ports, int32(8080))

				names := discovered.Names()
				assert.Contains(t, names, gwv1.SectionName("http"))
				assert.Contains(t, names, gwv1.SectionName("https"))
				assert.Contains(t, names, gwv1.SectionName("tcp"))
			}

			// Test non-existent lookups
			_, exists := discovered.GetByPort(9999)
			assert.False(t, exists)

			_, exists = discovered.GetByName("nonexistent")
			assert.False(t, exists)
		})
	}
}

func TestDiscoveredListeners_Methods(t *testing.T) {
	gateway := &gwv1.Gateway{
		Spec: gwv1.GatewaySpec{
			Listeners: []gwv1.Listener{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
		},
	}

	discovered := DiscoverListeners(gateway)

	// Test GetByPort
	dl, exists := discovered.GetByPort(80)
	assert.True(t, exists)
	assert.Equal(t, gwv1.SectionName("http"), dl.Name)

	_, exists = discovered.GetByPort(9999)
	assert.False(t, exists)

	// Test GetByName
	dl, exists = discovered.GetByName("https")
	assert.True(t, exists)
	assert.Equal(t, int32(443), dl.Port)

	_, exists = discovered.GetByName("nonexistent")
	assert.False(t, exists)

	// Test GetPortByName
	port, exists := discovered.GetPortByName("http")
	assert.True(t, exists)
	assert.Equal(t, int32(80), port)

	_, exists = discovered.GetPortByName("nonexistent")
	assert.False(t, exists)

	// Test GetNameByPort
	name, exists := discovered.GetNameByPort(443)
	assert.True(t, exists)
	assert.Equal(t, gwv1.SectionName("https"), name)

	_, exists = discovered.GetNameByPort(9999)
	assert.False(t, exists)

	// Test Ports and Names
	ports := discovered.Ports()
	assert.Len(t, ports, 2)
	assert.Contains(t, ports, int32(80))
	assert.Contains(t, ports, int32(443))

	names := discovered.Names()
	assert.Len(t, names, 2)
	assert.Contains(t, names, gwv1.SectionName("http"))
	assert.Contains(t, names, gwv1.SectionName("https"))
}
