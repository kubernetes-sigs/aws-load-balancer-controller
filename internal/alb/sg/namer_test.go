package sg

import "testing"

func TestNameSGs(t *testing.T) {
	namer := &namer{}
	for _, tc := range []struct {
		loadBalancerID         string
		expectedLBSGName       string
		expectedInstanceSGName string
	}{
		{
			loadBalancerID:         "abcdefgf1sh",
			expectedLBSGName:       "abcdefgf1sh",
			expectedInstanceSGName: "instance-abcdefgf1sh",
		},
	} {
		actualLBSGName := namer.NameLbSG(tc.loadBalancerID)
		if tc.expectedLBSGName != actualLBSGName {
			t.Errorf("expected:%v, actual:%v", tc.expectedLBSGName, actualLBSGName)
		}

		actualInstanceSGName := namer.NameInstanceSG(tc.loadBalancerID)
		if tc.expectedInstanceSGName != actualInstanceSGName {
			t.Errorf("expected:%v, actual:%v", tc.expectedInstanceSGName, actualInstanceSGName)
		}
	}
}
