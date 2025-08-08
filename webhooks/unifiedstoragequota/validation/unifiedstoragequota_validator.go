// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	v1 "k8s.io/api/admission/v1"
	"k8s.io/api/admission/v1beta1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/go-logr/logr"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/builder"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

const (
	webhookName            = "vmservice.cns.vsphere.vmware.com"
	vmWebhookPath          = "/getrequestedcapacityforvirtualmachine"
	vmSnapshotWebhookPath  = "/getrequestedcapacityforvirtualmachinesnapshot"
	scParamStoragePolicyID = "storagePolicyID"
)

// RequestedCapacity represents response body returned by this webhook to the SPQ webhook.
type RequestedCapacity struct {
	// Capacity represents the size to be reserved for the given resource
	Capacity resource.Quantity `json:"capacity"`

	// StorageClassName represents the StorageClass associated with the given resource
	StorageClassName string `json:"storageClassName"`

	// StoragePolicyID represents the StoragePolicyId associated with the given resource
	StoragePolicyID string `json:"storagePolicyId"`

	// Reason indicates the cause for returning capacity as 0
	Reason string `json:"reason,omitempty"`
}

// CapacityResponse is the response body returned by the webhook to the SPQ webhook.
// It has a list of RequestedCapacity for each storage class.
type CapacityResponse struct {
	RequestedCapacities []*RequestedCapacity
	admission.Response
}

// LegacyCapacityResponse is the response body returned by the webhook to the SPQ webhook.
// It has a single RequestedCapacity for the storage class.
// This is to be backward compatible with the previous behavior.
type LegacyCapacityResponse struct {
	RequestedCapacity
	admission.Response
}

type WebhookHandler interface {
	Handle(req admission.Request) CapacityResponse
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	WriteResponse(w http.ResponseWriter, response CapacityResponse)
}

type VMRequestedCapacityHandler struct {
	*pkgctx.WebhookContext
	admission.Decoder

	Client    client.Client
	Converter runtime.UnstructuredConverter
}

type VMSnapshotRequestedCapacityHandler struct {
	*pkgctx.WebhookContext
	admission.Decoder

	Client    client.Client
	Converter runtime.UnstructuredConverter
}

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	webhookNameLong := fmt.Sprintf("%s/%s/%s", ctx.Namespace, ctx.Name, webhookName)

	logger := ctx.Logger.WithName(webhookName)

	// Build the webhookContext.
	webhookContext := &pkgctx.WebhookContext{
		Context:            ctx,
		Name:               webhookName,
		Namespace:          ctx.Namespace,
		ServiceAccountName: ctx.ServiceAccountName,
		Recorder:           record.New(mgr.GetEventRecorderFor(webhookNameLong)),
		Logger:             logger,
	}
	// Initialize the webhook's decoder.
	decoder := admission.NewDecoder(mgr.GetScheme())

	vmHandler := &VMRequestedCapacityHandler{
		Client:         mgr.GetClient(),
		Converter:      runtime.DefaultUnstructuredConverter,
		Decoder:        decoder,
		WebhookContext: webhookContext,
	}

	mgr.GetWebhookServer().Register(vmWebhookPath, vmHandler)

	// Register the VMSnapshot webhook if VMSnapshots feature is enabled.
	if pkgcfg.FromContext(ctx).Features.VMSnapshots {
		logger.V(4).Info("VMSnapshots feature is enabled, registering VMSnapshot storage quota webhook")
		vmSnapshotHandler := &VMSnapshotRequestedCapacityHandler{
			Client:         mgr.GetClient(),
			Converter:      runtime.DefaultUnstructuredConverter,
			Decoder:        decoder,
			WebhookContext: webhookContext,
		}

		mgr.GetWebhookServer().Register(vmSnapshotWebhookPath, vmSnapshotHandler)
	}

	return nil
}

func (h *VMRequestedCapacityHandler) Handle(req admission.Request) CapacityResponse {
	var (
		obj, oldObj   *unstructured.Unstructured
		handleRequest func(ctx *pkgctx.WebhookRequestContext) CapacityResponse
	)

	if req.Operation == v1.Create || req.Operation == v1.Update {
		obj = &unstructured.Unstructured{}
		if err := h.DecodeRaw(req.Object, obj); err != nil {
			return CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)}
		}
	}

	if _, ok := obj.GetAnnotations()[pkgconst.SkipValidationAnnotationKey]; ok {
		// The VM has the skip validation annotation, so just allow this VM to
		// effectively bypass quota validation by returning 0 to the quota
		// framework.
		return CapacityResponse{Response: webhook.Allowed(builder.SkipValidationAllowed)}
	}

	if req.Operation == v1.Create {
		handleRequest = h.HandleCreate
	}
	if req.Operation == v1.Update {
		oldObj = &unstructured.Unstructured{}
		if err := h.DecodeRaw(req.OldObject, oldObj); err != nil {
			return CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)}
		}
		handleRequest = h.HandleUpdate
	}

	if obj == nil {
		return CapacityResponse{Response: webhook.Allowed(string(req.Operation))}
	}

	webhookRequestContext := &pkgctx.WebhookRequestContext{
		WebhookContext: h.WebhookContext,
		Op:             req.Operation,
		Obj:            obj,
		OldObj:         oldObj,
		UserInfo:       req.UserInfo,
		Logger:         h.WebhookContext.Logger.WithName(obj.GetNamespace()).WithName(obj.GetName()),
	}

	return handleRequest(webhookRequestContext)
}

// HandleCreate returns the Boot Disk capacity from the corresponding VMI/CVMI for the VM object in the AdmissionRequest.
func (h *VMRequestedCapacityHandler) HandleCreate(ctx *pkgctx.WebhookRequestContext) CapacityResponse {
	vm := &vmopv1.VirtualMachine{}
	if err := h.Converter.FromUnstructured(ctx.Obj.UnstructuredContent(), vm); err != nil {
		return CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)}
	}

	scName := vm.Spec.StorageClass

	sc := &storagev1.StorageClass{}
	if err := h.Client.Get(ctx, client.ObjectKey{Name: scName}, sc); err != nil {
		if apierrors.IsNotFound(err) {
			return CapacityResponse{Response: webhook.Errored(http.StatusNotFound, err)}
		}
		return CapacityResponse{Response: webhook.Errored(http.StatusInternalServerError, err)}
	}

	// This webhook will not be called if vm.Spec.Image is not set. This is done in order to ensure that imageless VMs
	// do not participate in quota validation. The VirtualMachine ValidatingWebhook still performs image validation.
	//
	// Please see https://github.com/vmware-tanzu/vm-operator/pull/822 and
	// https://github.com/vmware-tanzu/vm-operator/blob/main/config/webhook/storage_quota_webhook_configuration.yaml#L30-L31
	vmiName := vm.Spec.Image.Name
	var imageStatus vmopv1.VirtualMachineImageStatus

	switch vm.Spec.Image.Kind {
	case "VirtualMachineImage":
		vmi := &vmopv1.VirtualMachineImage{}
		if err := h.Client.Get(ctx, client.ObjectKey{Namespace: vm.Namespace, Name: vmiName}, vmi); err != nil {
			if apierrors.IsNotFound(err) {
				return CapacityResponse{Response: webhook.Errored(http.StatusNotFound, err)}
			}
			return CapacityResponse{Response: webhook.Errored(http.StatusInternalServerError, err)}
		}
		imageStatus = vmi.Status
	case "ClusterVirtualMachineImage":
		cvmi := &vmopv1.ClusterVirtualMachineImage{}
		if err := h.Client.Get(ctx, client.ObjectKey{Name: vmiName}, cvmi); err != nil {
			if apierrors.IsNotFound(err) {
				return CapacityResponse{Response: webhook.Errored(http.StatusNotFound, err)}
			}
			return CapacityResponse{Response: webhook.Errored(http.StatusInternalServerError, err)}
		}
		imageStatus = cvmi.Status
	default:
		return CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, fmt.Errorf("unsupported image kind %s", vm.Spec.Image.Kind))}
	}

	// Skip ISO images as they don't have boot disks.
	if imageStatus.Type == "ISO" {
		return CapacityResponse{Response: webhook.Allowed("")}
	}

	if len(imageStatus.Disks) < 1 {
		return CapacityResponse{Response: webhook.Errored(http.StatusNotFound, errors.New("no disks found in image status"))}
	}

	capacity := resource.NewQuantity(0, resource.BinarySI)
	// This is used to add zero to the total capacity for any disks in the image status that have nil capacity.
	emptyCapacity := resource.NewQuantity(0, resource.BinarySI)

	for _, disk := range imageStatus.Disks {
		if disk.Capacity == nil {
			capacity.Add(*emptyCapacity)
		} else {
			capacity.Add(*disk.Capacity)
		}
	}

	return CapacityResponse{
		RequestedCapacities: []*RequestedCapacity{
			{
				Capacity:         *capacity,
				StorageClassName: scName,
				// If this parameter does not exist, then it is not necessarily an error condition. Return
				// an empty value for StoragePolicyID and let Storage Policy Quota extension service decide
				// what to do.
				StoragePolicyID: sc.Parameters[scParamStoragePolicyID],
			},
		},
		Response: webhook.Allowed(""),
	}
}

// HandleUpdate checks for any positive difference in boot disk size and returns that difference.
//   - If both vm and oldVM have Spec.Advanced.BootDiskCapacity set, then only return a positive difference.
//   - If vm has Spec.Advanced.BootDiskCapacity set, while it is not set for oldVM, then use the first classic disk in
//     oldVM.Status.Volumes as this basis for comparison, again returning only a positive difference.
//   - If vm does not have Spec.Advanced.BootDiskCapacity set, then return an empty response.
func (h *VMRequestedCapacityHandler) HandleUpdate(ctx *pkgctx.WebhookRequestContext) CapacityResponse {
	if !ctx.Obj.GetDeletionTimestamp().IsZero() {
		return CapacityResponse{Response: admission.Allowed(builder.AdmitMesgUpdateOnDeleting)}
	}
	vm := &vmopv1.VirtualMachine{}
	if err := h.Converter.FromUnstructured(ctx.Obj.UnstructuredContent(), vm); err != nil {
		return CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)}
	}

	oldVM := &vmopv1.VirtualMachine{}
	if err := h.Converter.FromUnstructured(ctx.OldObj.UnstructuredContent(), oldVM); err != nil {
		return CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)}
	}

	var capacity, oldCapacity *resource.Quantity

	if vm.Spec.Advanced == nil || vm.Spec.Advanced.BootDiskCapacity == nil {
		return CapacityResponse{Response: webhook.Allowed("")}
	}

	capacity = vm.Spec.Advanced.BootDiskCapacity

	if oldVM.Spec.Advanced == nil || oldVM.Spec.Advanced.BootDiskCapacity == nil {
		oldCapacity = resource.NewQuantity(0, resource.BinarySI)
		for _, volume := range oldVM.Status.Volumes {
			if volume.Type == vmopv1.VirtualMachineStorageDiskTypeClassic {
				if volume.Limit != nil {
					oldCapacity = volume.Limit
					break
				}
			}
		}
	} else {
		oldCapacity = oldVM.Spec.Advanced.BootDiskCapacity
	}
	if capacity.Cmp(*oldCapacity) != 1 {
		return CapacityResponse{Response: webhook.Allowed("")}
	}
	capacity.Sub(*oldCapacity)
	scName := vm.Spec.StorageClass
	sc := &storagev1.StorageClass{}
	if err := h.Client.Get(ctx, client.ObjectKey{Name: scName}, sc); err != nil {
		if apierrors.IsNotFound(err) {
			return CapacityResponse{Response: webhook.Errored(http.StatusNotFound, err)}
		}
		return CapacityResponse{Response: webhook.Errored(http.StatusInternalServerError, err)}
	}
	return CapacityResponse{
		RequestedCapacities: []*RequestedCapacity{
			{
				Capacity:         *capacity,
				StorageClassName: scName,
				// If this parameter does not exist, then it is not necessarily an error condition. Return
				// an empty value for StoragePolicyID and let Storage Policy Quota extension service decide
				// what to do.
				StoragePolicyID: sc.Parameters[scParamStoragePolicyID],
			},
		},
		Response: webhook.Allowed(""),
	}
}

func (h *VMRequestedCapacityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil || r.Body == http.NoBody {
		err := errors.New("request body is empty")
		h.WebhookContext.Logger.Error(err, "bad request")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)})
		return
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)

	limitedReader := &io.LimitedReader{R: r.Body, N: maxRequestSize}
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		h.WebhookContext.Logger.Error(err, "unable to read the body from the incoming request")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)})
		return
	}
	if limitedReader.N <= 0 {
		err := fmt.Errorf("request entity is too large; limit is %d bytes", maxRequestSize)
		h.WebhookContext.Logger.Error(err, "unable to read the body from the incoming request; limit reached")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusRequestEntityTooLarge, err)})
		return
	}

	// verify the content type is accurate
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		err = fmt.Errorf("contentType=%s, expected application/json", contentType)
		h.WebhookContext.Logger.Error(err, "unable to process a request with unknown content type")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)})
		return
	}

	req := admission.Request{}
	ar := unversionedAdmissionReview{}
	// avoid an extra copy
	ar.Request = &req.AdmissionRequest
	ar.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("AdmissionReview"))
	_, _, err = admissionCodecs.UniversalDeserializer().Decode(body, nil, &ar)
	if err != nil {
		h.WebhookContext.Logger.Error(err, "unable to decode the request")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)})
		return
	}
	h.WebhookContext.Logger.V(4).Info("received request")

	h.WriteResponse(w, h.Handle(req))
}

var admissionScheme = runtime.NewScheme()
var admissionCodecs = serializer.NewCodecFactory(admissionScheme)

const maxRequestSize = int64(7 * 1024 * 1024)

func init() {
	utilruntime.Must(v1.AddToScheme(admissionScheme))
	utilruntime.Must(v1beta1.AddToScheme(admissionScheme))
}
func (h *VMRequestedCapacityHandler) WriteResponse(w http.ResponseWriter, response CapacityResponse) {
	if !response.Response.Allowed {
		h.WebhookContext.Logger.Error(errors.New(response.Response.Result.Message), "admission denied")
		// Write error and return early.
		http.Error(w, response.Response.Result.Message, int(response.Response.Result.Code))
		return
	}

	var res any
	// If VMSnapshots feature is enabled, then return []*RequestedCapacity.
	// Since that's what quota webhook expects when VMSnapshots feature is enabled.
	if pkgcfg.FromContext(h.WebhookContext.Context).Features.VMSnapshots {
		res = response.RequestedCapacities
	} else {
		switch len(response.RequestedCapacities) {
		case 1:
			// This is to be backward compatible with the previous behavior
			// when the VMsnapshot feature is disabled.
			res = LegacyCapacityResponse{
				RequestedCapacity: *response.RequestedCapacities[0],
				Response:          response.Response,
			}
		case 0:
			// This is to be backward compatible with the previous behavior.
			// This case should only happen when the VM Update doesn't change
			// the requested capacity.
			res = LegacyCapacityResponse{
				RequestedCapacity: RequestedCapacity{},
				Response:          response.Response,
			}
		default:
			// impossible case
		}
	}

	if err := json.NewEncoder(w).Encode(res); err != nil {
		h.WebhookContext.Logger.Error(err, "unable to encode and write the response")

		serverError := webhook.Errored(http.StatusInternalServerError, err)
		if err = json.NewEncoder(w).Encode(v1.AdmissionReview{Response: &serverError.AdmissionResponse}); err != nil {
			h.WebhookContext.Logger.Error(err, "still unable to encode and write the InternalServerError response")
		}
	}
}

func (h *VMSnapshotRequestedCapacityHandler) Handle(req admission.Request) CapacityResponse {
	if req.Operation != v1.Create {
		return CapacityResponse{Response: webhook.Allowed(string(req.Operation))}
	}

	obj := &unstructured.Unstructured{}
	if err := h.DecodeRaw(req.Object, obj); err != nil {
		return CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)}
	}

	if _, ok := obj.GetAnnotations()[pkgconst.SkipValidationAnnotationKey]; ok {
		// The VM Snapshot has the skip validation annotation, so just allow this VM Snapshot to
		// effectively bypass quota validation by returning 0 to the quota
		// framework.
		return CapacityResponse{Response: webhook.Allowed(builder.SkipValidationAllowed)}
	}

	webhookRequestContext := &pkgctx.WebhookRequestContext{
		WebhookContext: h.WebhookContext,
		Op:             req.Operation,
		Obj:            obj,
		UserInfo:       req.UserInfo,
		Logger:         h.WebhookContext.Logger.WithName(obj.GetNamespace()).WithName(obj.GetName()),
	}

	return h.HandleCreate(webhookRequestContext)

}

// HandleCreate returns the Boot Disk capacity from the corresponding VMI/CVMI for the VM object in the AdmissionRequest.
func (h *VMSnapshotRequestedCapacityHandler) HandleCreate(ctx *pkgctx.WebhookRequestContext) CapacityResponse {
	h.WebhookContext.Logger.V(4).Info("HandleCreate for VMSnapshot")

	vmSnapshot := &vmopv1.VirtualMachineSnapshot{}
	if err := h.Converter.FromUnstructured(ctx.Obj.UnstructuredContent(), vmSnapshot); err != nil {
		return CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)}
	}

	requested, err := kubeutil.CalculateReservedForSnapshotPerStorageClass(ctx, h.Client, h.WebhookContext.Logger, *vmSnapshot)
	if err != nil {
		return CapacityResponse{Response: webhook.Errored(http.StatusInternalServerError, err)}
	}

	requestedCapacities := make([]*RequestedCapacity, len(requested))
	for i, requested := range requested {
		cur := RequestedCapacity{
			Capacity:         *requested.Total,
			StorageClassName: requested.StorageClass,
		}
		// Fetch the storage class to get the storage policy ID.
		sc := &storagev1.StorageClass{}
		if err := h.Client.Get(ctx, client.ObjectKey{Name: requested.StorageClass}, sc); err != nil {
			if apierrors.IsNotFound(err) {
				h.WebhookContext.Logger.Error(err, "storage class not found", "storageClass", requested.StorageClass)
				return CapacityResponse{Response: webhook.Errored(http.StatusNotFound, err)}
			}
			return CapacityResponse{Response: webhook.Errored(http.StatusInternalServerError, err)}
		}
		cur.StoragePolicyID = sc.Parameters[scParamStoragePolicyID]
		requestedCapacities[i] = &cur
	}

	return CapacityResponse{
		RequestedCapacities: requestedCapacities,
		Response:            webhook.Allowed(""),
	}
}

func (h *VMSnapshotRequestedCapacityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serveHTTP(w, r, h, h.WebhookContext.Logger)
}

func serveHTTP(w http.ResponseWriter, r *http.Request, h WebhookHandler, logger logr.Logger) {
	if r.Body == nil || r.Body == http.NoBody {
		err := errors.New("request body is empty")
		logger.Error(err, "bad request")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)})
		return
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(r.Body)

	limitedReader := &io.LimitedReader{R: r.Body, N: maxRequestSize}
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		logger.Error(err, "unable to read the body from the incoming request")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)})
		return
	}
	if limitedReader.N <= 0 {
		err := fmt.Errorf("request entity is too large; limit is %d bytes", maxRequestSize)
		logger.Error(err, "unable to read the body from the incoming request; limit reached")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusRequestEntityTooLarge, err)})
		return
	}

	// verify the content type is accurate
	if contentType := r.Header.Get("Content-Type"); contentType != "application/json" {
		err = fmt.Errorf("contentType=%s, expected application/json", contentType)
		logger.Error(err, "unable to process a request with unknown content type")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)})
		return
	}

	req := admission.Request{}
	ar := unversionedAdmissionReview{}
	// avoid an extra copy
	ar.Request = &req.AdmissionRequest
	ar.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("AdmissionReview"))
	_, _, err = admissionCodecs.UniversalDeserializer().Decode(body, nil, &ar)
	if err != nil {
		logger.Error(err, "unable to decode the request")
		h.WriteResponse(w, CapacityResponse{Response: webhook.Errored(http.StatusBadRequest, err)})
		return
	}

	logger.V(4).Info("received request")

	h.WriteResponse(w, h.Handle(req))
}

func (h *VMSnapshotRequestedCapacityHandler) WriteResponse(w http.ResponseWriter, response CapacityResponse) {
	if !response.Response.Allowed {
		h.WebhookContext.Logger.Error(errors.New(response.Response.Result.Message), "admission denied")
		// Write error and return early.
		http.Error(w, response.Response.Result.Message, int(response.Response.Result.Code))
		return
	}

	// Only encode the RequestedCapacities since SPQ webhook only expects []*RequestedCapacity.
	if err := json.NewEncoder(w).Encode(response.RequestedCapacities); err != nil {
		h.WebhookContext.Logger.Error(err, "unable to encode and write the response")

		serverError := webhook.Errored(http.StatusInternalServerError, err)
		if err = json.NewEncoder(w).Encode(v1.AdmissionReview{Response: &serverError.AdmissionResponse}); err != nil {
			h.WebhookContext.Logger.Error(err, "still unable to encode and write the InternalServerError response")
		}
	}
}

// unversionedAdmissionReview is used to decode both v1 and v1beta1 AdmissionReview types.
type unversionedAdmissionReview struct {
	v1.AdmissionReview
}

var _ runtime.Object = &unversionedAdmissionReview{}
