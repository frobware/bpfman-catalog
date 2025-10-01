package manifests

import "fmt"

// NewImageDigestMirrorSet creates an IDMS for bpfman images
func NewImageDigestMirrorSet(shortDigest string) *ImageDigestMirrorSet {
	name := "fbc-bpfman-idms"
	if shortDigest != "" {
		name = fmt.Sprintf("fbc-bpfman-idms-%s", shortDigest)
	}

	return &ImageDigestMirrorSet{
		TypeMeta: TypeMeta{
			APIVersion: "config.openshift.io/v1",
			Kind:       "ImageDigestMirrorSet",
		},
		ObjectMeta: ObjectMeta{
			Name: name,
		},
		Spec: ImageDigestMirrorSetSpec{
			ImageDigestMirrors: []ImageDigestMirror{
				{
					Source: "registry.redhat.io/bpfman/bpfman-agent",
					Mirrors: []string{
						"quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-agent",
					},
				},
				{
					Source: "registry.redhat.io/bpfman/bpfman-operator-bundle",
					Mirrors: []string{
						"quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator-bundle",
					},
				},
				{
					Source: "registry.redhat.io/bpfman/bpfman-rhel9-operator",
					Mirrors: []string{
						"quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman-operator",
					},
				},
				{
					Source: "registry.redhat.io/bpfman/bpfman",
					Mirrors: []string{
						"quay.io/redhat-user-workloads/ocp-bpfman-tenant/ocp-bpfman",
					},
				},
			},
		},
	}
}
