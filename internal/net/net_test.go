/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package net

import (
	"net"
	"testing"
)

func TestIsPortAvailable(t *testing.T) {
	if !IsPortAvailable(0) {
		t.Fatal("expected port 0 to be available (random port) but returned false")
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer ln.Close()

	p := ln.Addr().(*net.TCPAddr).Port
	if IsPortAvailable(p) {
		t.Fatalf("expected port %v to not be available", p)
	}
}
