package udp

import (
	"fmt"
	"github.com/pkg/errors"
	"net"
	"time"
)

const (
	maxTries                      = 100
	consecutiveSuccessesThreshold = 5
)

// Verifier is responsible for verify the behavior of an HTTP endpoint.
type Verifier interface {
	VerifyUDP(endpoint string) error
}

func NewDefaultVerifier() *defaultVerifier {
	return &defaultVerifier{}
}

var _ Verifier = &defaultVerifier{}

// default implementation for Verifier.
type defaultVerifier struct {
}

func (v *defaultVerifier) VerifyUDP(endpoint string) error {
	tries := 0
	consecutiveSuccesses := 0
	for tries < maxTries {
		err := v.doCall(endpoint)
		if err == nil {
			consecutiveSuccesses++
		} else {
			fmt.Printf("Got an error during UDP call (%+v\n)", err)
		}
		if consecutiveSuccesses >= consecutiveSuccessesThreshold {
			return nil
		}
		tries++
		time.Sleep(1 * time.Second)
	}
	return errors.New("Unable to observe stable UDP connection")
}

func (v *defaultVerifier) doCall(endpoint string) error {
	serverAddr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	message := []byte("Hello, UDP Server!")
	_, err = conn.Write(message)
	if err != nil {
		return err
	}

	// Set a read deadline to avoid blocking indefinitely
	err = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		return err
	}

	buffer := make([]byte, 1024)
	_, _, err = conn.ReadFromUDP(buffer)
	if err != nil {
		return err
	}

	return nil
}
