package utils

import (
	"janus-ingress-controller/internal/types"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NamespacedName(obj client.Object) k8stypes.NamespacedName {
	return k8stypes.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func NamespacedNameKind(obj client.Object) types.NamespacedNameKind {
	return types.NamespacedNameKind{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Kind:      obj.GetObjectKind().GroupVersionKind().Kind,
	}
}
