package types

import (
	"fmt"
	"strings"

	k8stypes "k8s.io/apimachinery/pkg/types"
)

type NamespacedNameKind struct {
	Namespace string
	Name      string
	Kind      string
}

func (n NamespacedNameKind) NamespacedName() k8stypes.NamespacedName {
	return k8stypes.NamespacedName{
		Namespace: n.Namespace,
		Name:      n.Name,
	}
}

func (n NamespacedNameKind) String() string {
	return n.Kind + "/" + n.Namespace + "/" + n.Name
}

func (n NamespacedNameKind) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

func (n *NamespacedNameKind) UnmarshalText(text []byte) error {
	return n.FromString(string(text))
}

func (n *NamespacedNameKind) FromString(s string) error {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid format for NamespacedNameKind: %q, expected Kind/Namespace/Name", s)
	}

	n.Kind = parts[0]
	n.Namespace = parts[1]
	n.Name = parts[2]
	return nil
}
