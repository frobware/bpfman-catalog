package manifests

import "fmt"

// NewNamespace creates a namespace manifest with monitoring enabled
func NewNamespace(baseName, shortDigest string) *Namespace {
	name := baseName
	if shortDigest != "" {
		name = fmt.Sprintf("%s-%s", baseName, shortDigest)
	}

	return &Namespace{
		TypeMeta: TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"openshift.io/cluster-monitoring": "true",
			},
		},
	}
}
