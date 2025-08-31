package status

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"janus-ingress-controller/internal/types"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sync"
)

const UpdateChannelBufferSize = 1000

var cmpIgnoreLastTransitionTime = cmp.Options{
	cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	cmpopts.IgnoreMapEntries(func(k string, _ any) bool {
		return k == "lastTransitionTime"
	}),
}

type Update struct {
	NamespacedName k8stypes.NamespacedName
	Resource       client.Object
	Mutator        Mutator
}

type Updater interface {
	Update(u Update)
}

type Mutator interface {
	Mutate(obj client.Object) client.Object
}

type MutatorFunc func(client.Object) client.Object

func (m MutatorFunc) Mutate(obj client.Object) client.Object {
	if m == nil {
		return nil
	}

	return m(obj)
}

type UpdateHandler struct {
	log           logr.Logger
	client        client.Client
	updateChannel chan Update
	wg            *sync.WaitGroup
}

func (u *UpdateHandler) Writer() Updater {
	return &UpdateWriter{
		updateChannel: u.updateChannel,
		wg:            u.wg,
	}
}

type UpdateWriter struct {
	updateChannel chan<- Update
	wg            *sync.WaitGroup
}

func (u *UpdateWriter) Update(update Update) {
	u.wg.Wait()
	u.updateChannel <- update
}

func NewStatusUpdateHandler(log logr.Logger, client client.Client) *UpdateHandler {
	u := &UpdateHandler{
		log:           log,
		client:        client,
		updateChannel: make(chan Update, UpdateChannelBufferSize),
		wg:            new(sync.WaitGroup),
	}

	u.wg.Add(1)
	return u
}

func (u *UpdateHandler) Start(ctx context.Context) error {
	u.log.Info("started status update handler")
	defer u.log.Info("stopped status update handler")

	u.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return nil
		case update := <-u.updateChannel:
			u.log.V(1).Info("received a status update", "namespace", update.NamespacedName.Namespace,
				"name", update.NamespacedName.Name,
				"kind", types.KindOf(update.Resource),
			)

			u.apply(ctx, update)
		}
	}
}

func (u *UpdateHandler) NeedsLeaderElection() bool {
	return true
}

func (u *UpdateHandler) apply(ctx context.Context, update Update) {
	if err := retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return k8serrors.IsConflict(err)
	}, func() error {
		return u.updateStatus(ctx, update)
	}); err != nil {
		u.log.Error(err, "unable to update status", "name", update.NamespacedName.Name,
			"namespace", update.NamespacedName.Namespace)
	}
}

func (u *UpdateHandler) updateStatus(ctx context.Context, update Update) error {
	var obj = update.Resource
	if err := u.client.Get(ctx, update.NamespacedName, obj); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	newObj := update.Mutator.Mutate(obj)
	if newObj == nil {
		return nil
	}

	if statusEqual(obj, newObj, cmpIgnoreLastTransitionTime) {
		u.log.V(1).Info("status is equal, skipping update", "name", update.NamespacedName.Name,
			"namespace", update.NamespacedName.Namespace,
			"kind", types.KindOf(obj))
		return nil
	}

	newObj.SetUID(obj.GetUID())

	u.log.Info("updating status", "name", update.NamespacedName.Name,
		"namespace", update.NamespacedName.Namespace,
		"kind", types.KindOf(newObj),
	)

	return u.client.Status().Update(ctx, newObj)
}

func statusEqual(a, b any, opts ...cmp.Option) bool {
	var statusA, statusB any

	switch a := a.(type) {
	case *gatewayv1.GatewayClass:
		b, ok := b.(*gatewayv1.GatewayClass)
		if !ok {
			return false
		}
		statusA, statusB = a.Status, b.Status

	case *gatewayv1.Gateway:
		b, ok := b.(*gatewayv1.Gateway)
		if !ok {
			return false
		}
		statusA, statusB = a.Status, b.Status
	default:
		return false
	}

	return cmp.Equal(statusA, statusB, opts...)
}
