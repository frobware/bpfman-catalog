package manifests

import "fmt"

// NewOperatorGroup creates an OperatorGroup manifest
func NewOperatorGroup(namespace, shortDigest string) *OperatorGroup {
	name := "bpfman"
	if shortDigest != "" {
		name = fmt.Sprintf("bpfman-%s", shortDigest)
	}

	return &OperatorGroup{
		TypeMeta: TypeMeta{
			APIVersion: "operators.coreos.com/v1",
			Kind:       "OperatorGroup",
		},
		ObjectMeta: ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: OperatorGroupSpec{},
	}
}
