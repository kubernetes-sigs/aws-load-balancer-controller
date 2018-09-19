package cache

import "testing"

func Test_Cachable(t *testing.T) {
	if !cachable("DescribeTags") {
		t.Errorf("DescribeTags should be cachable")
	}
	if !cachable("ListTags") {
		t.Errorf("ListTags should be cachable")
	}
	if !cachable("GetSubnets") {
		t.Errorf("GetSubnets should be cachable")
	}
	if cachable("CreateTags") {
		t.Errorf("CreateTags should not be cachable")
	}
}
