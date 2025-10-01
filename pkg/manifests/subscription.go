package manifests

import "fmt"

// NewSubscription creates a Subscription manifest
func NewSubscription(namespace, catalogSourceName, shortDigest string) *Subscription {
	name := "bpfman-operator"
	if shortDigest != "" {
		name = fmt.Sprintf("bpfman-operator-%s", shortDigest)
	}

	return &Subscription{
		TypeMeta: TypeMeta{
			APIVersion: "operators.coreos.com/v1alpha1",
			Kind:       "Subscription",
		},
		ObjectMeta: ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: SubscriptionSpec{
			Channel:             "stable",
			Name:                "bpfman-operator", // Must match packageName from FBC
			Source:              catalogSourceName,
			SourceNamespace:     "openshift-marketplace",
			InstallPlanApproval: "Automatic",
		},
	}
}
