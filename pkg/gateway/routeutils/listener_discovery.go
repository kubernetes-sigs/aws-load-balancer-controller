package routeutils

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// DiscoveredListener represents a listener with its index for efficient access
type DiscoveredListener struct {
	Listener gwv1.Listener
	Index    int
	Port     int32
	Name     gwv1.SectionName
}

// DiscoveredListeners holds all discovered listeners with efficient lookup maps
type DiscoveredListeners struct {
	All        []DiscoveredListener
	ByPort     map[int32]DiscoveredListener
	ByName     map[gwv1.SectionName]DiscoveredListener
	PortToName map[int32]gwv1.SectionName
}

// DiscoverListeners extracts and indexes all listeners from a Gateway once
func DiscoverListeners(gw *gwv1.Gateway) *DiscoveredListeners {
	discovered := &DiscoveredListeners{
		All:        make([]DiscoveredListener, 0, len(gw.Spec.Listeners)),
		ByPort:     make(map[int32]DiscoveredListener),
		ByName:     make(map[gwv1.SectionName]DiscoveredListener),
		PortToName: make(map[int32]gwv1.SectionName),
	}

	for i, listener := range gw.Spec.Listeners {
		port := int32(listener.Port)
		dl := DiscoveredListener{
			Listener: listener,
			Index:    i,
			Port:     port,
			Name:     listener.Name,
		}

		discovered.All = append(discovered.All, dl)
		discovered.ByPort[port] = dl
		discovered.ByName[listener.Name] = dl
		discovered.PortToName[port] = listener.Name
	}

	return discovered
}

// GetByPort returns a listener by port number
func (d *DiscoveredListeners) GetByPort(port int32) (DiscoveredListener, bool) {
	listener, exists := d.ByPort[port]
	return listener, exists
}

// GetByName returns a listener by section name
func (d *DiscoveredListeners) GetByName(name gwv1.SectionName) (DiscoveredListener, bool) {
	listener, exists := d.ByName[name]
	return listener, exists
}

// GetPortByName returns the port for a given listener name
func (d *DiscoveredListeners) GetPortByName(name gwv1.SectionName) (int32, bool) {
	if listener, exists := d.ByName[name]; exists {
		return listener.Port, true
	}
	return 0, false
}

// GetNameByPort returns the listener name for a given port
func (d *DiscoveredListeners) GetNameByPort(port int32) (gwv1.SectionName, bool) {
	name, exists := d.PortToName[port]
	return name, exists
}

// Ports returns all listener ports
func (d *DiscoveredListeners) Ports() []int32 {
	ports := make([]int32, 0, len(d.ByPort))
	for port := range d.ByPort {
		ports = append(ports, port)
	}
	return ports
}

// Names returns all listener names
func (d *DiscoveredListeners) Names() []gwv1.SectionName {
	names := make([]gwv1.SectionName, 0, len(d.ByName))
	for name := range d.ByName {
		names = append(names, name)
	}
	return names
}
