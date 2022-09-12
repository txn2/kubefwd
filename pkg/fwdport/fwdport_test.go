package fwdport

import "testing"

func Test_sanitizeHost(t *testing.T) {
	tests := []struct {
		contextName string
		want        string
	}{
		{ // This is how Openshift generates context names
			contextName: "service-name.namespace.project-name/cluster-name:6443/username",
			want:        "service-name-namespace-project-name-cluster-name-6443-username",
		},
		{contextName: "-----test-----", want: "test"},
	}
	for _, tt := range tests {
		t.Run("Sanitize hostname generated from context and namespace: "+tt.contextName, func(t *testing.T) {
			if got := sanitizeHost(tt.contextName); got != tt.want {
				t.Errorf("sanitizeHost() = %v, want %v", got, tt.want)
			}
		})
	}
}
