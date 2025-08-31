package controller

import (
	"context"
	"errors"
	"fmt"
	"janus-ingress-controller/api/v1"
	"janus-ingress-controller/internal/controller/indexer"
	"janus-ingress-controller/internal/controller/status"
	"janus-ingress-controller/internal/utils"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// GatewayReconciler reconciles a Gateway object.
type GatewayReconciler struct { //nolint:revive
	client.Client
	Scheme  *runtime.Scheme
	Log     logr.Logger
	Updater status.Updater
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	bdr := ctrl.NewControllerManagedBy(mgr).
		For(
			&gatewayv1.Gateway{},
			builder.WithPredicates(
				predicate.NewPredicateFuncs(r.checkGatewayClass),
			),
		).
		WithEventFilter(
			predicate.Or(
				predicate.GenerationChangedPredicate{},
				predicate.NewPredicateFuncs(TypePredicate[*corev1.Secret]()),
			),
		).
		Watches(
			&gatewayv1.GatewayClass{},
			handler.EnqueueRequestsFromMapFunc(r.listGatewayForGatewayClass),
			builder.WithPredicates(
				predicate.NewPredicateFuncs(r.matchesGatewayClass),
			),
		).
		Watches(
			&v1.GatewayProxy{},
			handler.EnqueueRequestsFromMapFunc(r.listGatewaysForGatewayProxy),
		)

	if GetEnableReferenceGrant() {
		bdr.Watches(&v1beta1.ReferenceGrant{},
			handler.EnqueueRequestsFromMapFunc(r.listReferenceGrantsForGateway),
			builder.WithPredicates(referenceGrantPredicates(KindGateway)),
		)
	}

	return bdr.Complete(r)
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	gateway := new(gatewayv1.Gateway)
	if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
		if client.IgnoreNotFound(err) == nil {
			gateway.Namespace = req.Namespace
			gateway.Name = req.Name

			gateway.TypeMeta = metav1.TypeMeta{
				Kind:       KindGateway,
				APIVersion: gatewayv1.GroupVersion.String(),
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	conditionProgrammedStatus, conditionProgrammedMsg := true, "Programmed"

	r.Log.Info("gateway has been accepted", "gateway", gateway.GetName())
	type conditionStatus struct {
		status bool
		msg    string
	}
	acceptStatus := conditionStatus{
		status: true,
		msg:    acceptedMessage("gateway"),
	}

	r.processListenerConfig(gateway)
	if err := r.processInfrastructure(gateway); err != nil {
		acceptStatus = conditionStatus{
			status: false,
			msg:    err.Error(),
		}
	}

	listenerStatuses, err := getListenerStatus(ctx, r.Client, gateway)
	if err != nil {
		r.Log.Error(err, "failed to get listener status", "gateway", req.NamespacedName)
		return ctrl.Result{}, err
	}

	accepted := SetGatewayConditionAccepted(gateway, acceptStatus.status, acceptStatus.msg)
	programmed := SetGatewayConditionProgrammed(gateway, conditionProgrammedStatus, conditionProgrammedMsg)
	if accepted || programmed || len(listenerStatuses) > 0 {
		if len(listenerStatuses) > 0 {
			gateway.Status.Listeners = listenerStatuses
		}

		r.Updater.Update(status.Update{
			NamespacedName: utils.NamespacedName(gateway),
			Resource:       &gatewayv1.Gateway{},
			Mutator: status.MutatorFunc(func(obj client.Object) client.Object {
				t, ok := obj.(*gatewayv1.Gateway)
				if !ok {
					err := fmt.Errorf("unsupported object type %T", obj)
					panic(err)
				}
				tCopy := t.DeepCopy()
				tCopy.Status = gateway.Status
				return tCopy
			}),
		})

		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *GatewayReconciler) matchesGatewayClass(obj client.Object) bool {
	gateway, ok := obj.(*gatewayv1.GatewayClass)
	if !ok {
		r.Log.Error(fmt.Errorf("unexpected object type"), "failed to convert object to Gateway")
		return false
	}
	return matchesController(string(gateway.Spec.ControllerName))
}

func (r *GatewayReconciler) listGatewayForGatewayClass(ctx context.Context, gatewayClass client.Object) []reconcile.Request {
	gatewayList := &gatewayv1.GatewayList{}
	if err := r.List(context.Background(), gatewayList); err != nil {
		r.Log.Error(err, "failed to list gateways for gateway class",
			"gatewayclass", gatewayClass.GetName(),
		)
		return nil
	}

	return reconcileGatewaysMatchGatewayClass(gatewayClass, gatewayList.Items)
}

func (r *GatewayReconciler) checkGatewayClass(obj client.Object) bool {
	gateway := obj.(*gatewayv1.Gateway)
	gatewayClass := &gatewayv1.GatewayClass{}
	if err := r.Get(context.Background(), client.ObjectKey{Name: string(gateway.Spec.GatewayClassName)}, gatewayClass); err != nil {
		r.Log.Error(err, "failed to get gateway class", "gateway", gateway.GetName(), "gatewayclass", gateway.Spec.GatewayClassName)
		return false
	}

	return matchesController(string(gatewayClass.Spec.ControllerName))
}

func (r *GatewayReconciler) listGatewaysForGatewayProxy(ctx context.Context, obj client.Object) []reconcile.Request {
	gatewayProxy, ok := obj.(*v1.GatewayProxy)
	if !ok {
		r.Log.Error(fmt.Errorf("unexpected object type"), "failed to convert object to GatewayProxy")
		return nil
	}
	namespace := gatewayProxy.GetNamespace()
	name := gatewayProxy.GetName()

	gatewayList := &gatewayv1.GatewayList{}
	if err := r.List(ctx, gatewayList, client.MatchingFields{
		indexer.ParametersRef: indexer.GenIndexKey(namespace, name),
	}); err != nil {
		r.Log.Error(err, "failed to list gateways for gateway proxy", "gatewayproxy", gatewayProxy.GetName())
		return nil
	}

	recs := make([]reconcile.Request, 0, len(gatewayList.Items))
	for _, gateway := range gatewayList.Items {
		if !r.checkGatewayClass(&gateway) {
			continue
		}
		recs = append(recs, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: gateway.GetNamespace(),
				Name:      gateway.GetName(),
			},
		})
	}
	return recs
}

func (r *GatewayReconciler) listGatewaysForHTTPRoute(ctx context.Context, obj client.Object) []reconcile.Request {
	httpRoute, ok := obj.(*gatewayv1.HTTPRoute)
	if !ok {
		r.Log.Error(
			fmt.Errorf("unexpected object type"),
			"HTTPRoute watch predicate received unexpected object type",
			"expected", "*gatewayapi.HTTPRoute", "found", reflect.TypeOf(obj),
		)
		return nil
	}
	recs := []reconcile.Request{}
	for _, routeParentStatus := range httpRoute.Status.Parents {
		gatewayNamespace := httpRoute.GetNamespace()
		parentRef := routeParentStatus.ParentRef
		if parentRef.Group != nil && *parentRef.Group != gatewayv1.GroupName {
			continue
		}
		if parentRef.Kind != nil && *parentRef.Kind != "Gateway" {
			continue
		}
		if parentRef.Namespace != nil {
			gatewayNamespace = string(*parentRef.Namespace)
		}

		gateway := new(gatewayv1.Gateway)
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: gatewayNamespace,
			Name:      string(parentRef.Name),
		}, gateway); err != nil {
			continue
		}

		if !r.checkGatewayClass(gateway) {
			continue
		}

		recs = append(recs, reconcile.Request{
			NamespacedName: client.ObjectKey{
				Namespace: gatewayNamespace,
				Name:      string(parentRef.Name),
			},
		})
	}
	return recs
}

func (r *GatewayReconciler) listReferenceGrantsForGateway(ctx context.Context, obj client.Object) (requests []reconcile.Request) {
	grant, ok := obj.(*v1beta1.ReferenceGrant)
	if !ok {
		r.Log.Error(
			errors.New("unexpected object type"),
			"ReferenceGrant watch predicate received unexpected object type",
			"expected", FullTypeName(new(v1beta1.ReferenceGrant)), "found", FullTypeName(obj),
		)
		return nil
	}

	var gatewayList gatewayv1.GatewayList
	if err := r.List(ctx, &gatewayList); err != nil {
		r.Log.Error(err, "failed to list gateways in watch predicate", "ReferenceGrant", grant.GetName())
		return nil
	}

	for _, gateway := range gatewayList.Items {
		gw := v1beta1.ReferenceGrantFrom{
			Group:     gatewayv1.GroupName,
			Kind:      KindGateway,
			Namespace: v1beta1.Namespace(gateway.GetNamespace()),
		}
		for _, from := range grant.Spec.From {
			if from == gw {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: gateway.GetNamespace(),
						Name:      gateway.GetName(),
					},
				})
			}
		}
	}
	return requests
}

func (r *GatewayReconciler) processInfrastructure(gateway *gatewayv1.Gateway) error {
	return ProcessGatewayProxy(r.Client, r.Log, gateway, utils.NamespacedNameKind(gateway))
}

func (r *GatewayReconciler) processListenerConfig(gateway *gatewayv1.Gateway) {
	listeners := gateway.Spec.Listeners
	for _, listener := range listeners {
		if listener.TLS == nil || listener.TLS.CertificateRefs == nil {
			continue
		}
		secret := corev1.Secret{}
		for _, ref := range listener.TLS.CertificateRefs {
			ns := gateway.GetNamespace()
			if ref.Namespace != nil {
				ns = string(*ref.Namespace)
			}
			if ref.Kind != nil && *ref.Kind == KindSecret {
				if err := r.Get(context.Background(), client.ObjectKey{
					Namespace: ns,
					Name:      string(ref.Name),
				}, &secret); err != nil {
					r.Log.Error(err, "failed to get secret", "namespace", ns, "name", ref.Name)
					SetGatewayListenerConditionProgrammed(gateway, string(listener.Name), false, err.Error())
					SetGatewayListenerConditionResolvedRefs(gateway, string(listener.Name), false, err.Error())
					break
				}
			}
		}
	}
}
