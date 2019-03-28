package ctl

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	kvv1 "kubevirt.io/kubevirt/pkg/api/v1"
)

var log = logf.Log.WithName("controller_cnv-svc4vmi")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Dummy Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &Reconciler{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cnv-svc4vmi-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Vmi with below predicate
	chck := func(vmi *kvv1.VirtualMachineInstance) bool {
		return vmi.Labels["kubevirt.io/svc"] == "true"
	}

	pred := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			vmi := e.Object.DeepCopyObject().(*kvv1.VirtualMachineInstance)
			return chck(vmi)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			vmio := e.ObjectOld.DeepCopyObject().(*kvv1.VirtualMachineInstance)
			vmin := e.ObjectNew.DeepCopyObject().(*kvv1.VirtualMachineInstance)
			return (chck(vmio) || chck(vmin)) &&
				vmio.Labels["kubevirt.io/svc"] != vmin.Labels["kubevirt.io/svc"]
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	err = c.Watch(&source.Kind{Type: &kvv1.VirtualMachineInstance{}}, &handler.EnqueueRequestForObject{}, pred)
	if err != nil {
		return err
	}

	pred = predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner CnvPod
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kvv1.VirtualMachineInstance{},
	}, pred)
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &Reconciler{}

// Reconciler reconciler
type Reconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CnvPod object and makes changes based on the state read
// and what is in the CnvPod.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *Reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	reqLogger := log.WithValues("Namespace", request.Namespace, "Name", request.Name)
	reqLogger.Info("Reconciling Vmi")

	// Fetch the Vmi instance
	vmi := &kvv1.VirtualMachineInstance{}
	err := r.client.Get(context.TODO(), request.NamespacedName, vmi)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Create a new Service object - not create it yet
	svc, err := r.newSvcForVmi(vmi)
	if err != nil {
		return reconcile.Result{}, err
	}
	// Check if the Service exists
	svcfound := &corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, svcfound)

	if err != nil {
		if errors.IsNotFound(err) { //service not found
			if vmi.Labels["kubevirt.io/svc"] == "true" { //only with this label
				reqLogger.Info("Creating Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
				if err = r.client.Create(context.TODO(), svc); err != nil {
					return reconcile.Result{}, err
				} //else don't requeue
			}
		} else {
			return reconcile.Result{}, err //other error
		}
	} else { //Service found
		if vmi.Labels["kubevirt.io/svc"] != "true" { // Check if to delete the Service
			reqLogger.Info("Deleting Service", "Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			if err = r.client.Delete(context.TODO(), svc); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

// newSvcForVmi returns a service with the same name/namespace as the Vmi
func (r *Reconciler) newSvcForVmi(vmi *kvv1.VirtualMachineInstance) (*corev1.Service, error) {

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmi.Name,
			Namespace: vmi.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "ssh",
					Protocol: "TCP",
					Port:     22,
					//TargetPort: 22,
				},
			},
			Selector: map[string]string{
				"kubevirt.io/created-by": fmt.Sprintf("%s", vmi.UID),
				//"kubevirt.io/svc": vmi.Name,
			},
			Type: v1.ServiceTypeNodePort,
		},
	}

	// Set pod instance as the owner and controller
	err := controllerutil.SetControllerReference(vmi, svc, r.scheme)
	return svc, err
}

/*
	// Fetch the Vmi pod virt-lancher by uid and update the label kubirt.io/svc
	opts := &client.ListOptions{}
	opts.SetLabelSelector(fmt.Sprintf("kubevirt.io/created-by=%s", vmi.UID))
	opts.InNamespace(request.NamespacedName.Namespace)

	podList := &corev1.PodList{}
	err = r.client.List(context.TODO(), opts, podList)
	if err != nil {
		reqLogger.Info("Unable to find virt-launcher pod for Vmi", "Vmi.Namespace", vmi.Namespace, "Vmi.Name", vmi.Name)
		return reconcile.Result{}, err
	}
	for _, v := range podList.Items {
		pod := &corev1.Pod{}
		if err := r.client.Get(context.TODO(), types.NamespacedName{Name: v.GetName(), Namespace: v.GetNamespace()}, pod); err != nil {
			return reconcile.Result{}, err
		}
		if strings.HasPrefix(pod.Name, "virt-launcher-") &&
			pod.Labels["kubevirt.io/svc"] != svc.Spec.Selector["kubevirt.io/svc"] {
			pod.Labels["kubevirt.io/svc"] = svc.Spec.Selector["kubevirt.io/svc"]
			if err = r.client.Update(context.TODO(), pod); err != nil {
				reqLogger.Error(err, "Failed to update Pod", "Pod.Namespace", pod.Namespace, "Pod.Name", pod.Name)
				return reconcile.Result{}, err
			}
		}
	}
*/

/*
func createComponent(err error, obj runtime.Object, kind string, c client.Client) (reconcile.Result, error) {
	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating a new " + kind)
		err = c.Create(context.TODO(), obj)
		if err != nil {
			return reconcile.Result{}, err
		}
		// Object CR created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}
	// Object CR already exists - don't requeue
	log.Info("Skip reconcile: " + kind + " already exists")

	return reconcile.Result{}, nil
}
*/
