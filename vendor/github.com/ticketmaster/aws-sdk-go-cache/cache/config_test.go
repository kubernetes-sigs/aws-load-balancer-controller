package cache

import "testing"

func Test_Cachable(t *testing.T) {
	if !isCachable("DescribeTags") {
		t.Errorf("DescribeTags should be isCachable")
	}
	if !isCachable("ListTags") {
		t.Errorf("ListTags should be isCachable")
	}
	if !isCachable("GetSubnets") {
		t.Errorf("GetSubnets should be isCachable")
	}
	if isCachable("CreateTags") {
		t.Errorf("CreateTags should not be isCachable")
	}
}
