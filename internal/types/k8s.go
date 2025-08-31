package types

import (
	"janus-ingress-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	KindGateway      = "Gateway"
	KindGatewayClass = "GatewayClass"
	KindIngress      = "Ingress"
	KindSecret       = "Secret"
	KindIngressClass = "IngressClass"
	KindGatewayProxy = "GatewayProxy"
)

func KindOf(obj any) string {
	switch obj.(type) {
	case *gatewayv1.Gateway:
		return KindGateway
	case *gatewayv1.GatewayClass:
		return KindGatewayClass
	case *netv1.Ingress:
		return KindIngress
	case *netv1.IngressClass:
		return KindIngressClass
	case *corev1.Secret:
		return KindSecret
	case *v1.GatewayProxy:
		return KindGatewayProxy
	default:
		return "Unknown"
	}
}
