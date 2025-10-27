// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller/priorityqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

var _ = Describe("EventType", func() {
	Context("String", func() {
		It("should return 'create' for EventCreate", func() {
			Expect(kubeutil.EventCreate.String()).To(Equal("create"))
		})

		It("should return 'update' for EventUpdate", func() {
			Expect(kubeutil.EventUpdate.String()).To(Equal("update"))
		})

		It("should return 'delete' for EventDelete", func() {
			Expect(kubeutil.EventDelete.String()).To(Equal("delete"))
		})

		It("should return 'generic' for EventGeneric", func() {
			Expect(kubeutil.EventGeneric.String()).To(Equal("generic"))
		})

		It("should return 'unknown' for unknown event type", func() {
			var unknownEvent kubeutil.EventType = 255
			Expect(unknownEvent.String()).To(Equal("unknown"))
		})
	})
})

var _ = Describe("TypedEnqueueRequestForObject", func() {
	var (
		ctx     context.Context
		handler *kubeutil.TypedEnqueueRequestForObject[*corev1.ConfigMap]
		queue   workqueue.TypedRateLimitingInterface[reconcile.Request]
		cm      *corev1.ConfigMap
	)

	BeforeEach(func() {
		ctx = log.IntoContext(context.Background(), log.Log)
		handler = &kubeutil.TypedEnqueueRequestForObject[*corev1.ConfigMap]{
			Logger: log.Log,
		}
		queue = workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-cm",
				Namespace:       "test-ns",
				ResourceVersion: "1",
			},
		}
	})

	AfterEach(func() {
		if queue != nil {
			queue.ShutDown()
		}
	})

	Context("Create", func() {
		It("should enqueue create event with valid object", func() {
			evt := event.TypedCreateEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Create(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			item, _ := queue.Get()
			Expect(item.Name).To(Equal("test-cm"))
			Expect(item.Namespace).To(Equal("test-ns"))
		})

		It("should not enqueue create event with nil object", func() {
			evt := event.TypedCreateEvent[*corev1.ConfigMap]{
				Object: nil,
			}
			handler.Create(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(0))
		})

		It("should use custom priority function when provided", func() {
			customPriorityCallCount := 0
			handler.GetPriority = func(
				ctx context.Context,
				eventType kubeutil.EventType,
				newObj, oldObj *corev1.ConfigMap,
				defaultPriority int) int {
				customPriorityCallCount++
				Expect(eventType).To(Equal(kubeutil.EventCreate))
				return defaultPriority + 100
			}

			evt := event.TypedCreateEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Create(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			// Custom priority function is only called for priority queues
			// For regular queues, it's not called
			Expect(customPriorityCallCount).To(Equal(0))
		})

		It("should handle IsInInitialList flag", func() {
			evt := event.TypedCreateEvent[*corev1.ConfigMap]{
				Object:          cm,
				IsInInitialList: true,
			}
			handler.Create(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
		})
	})

	Context("Update", func() {
		It("should enqueue update event with valid new object", func() {
			oldCM := cm.DeepCopy()
			oldCM.ResourceVersion = "0"

			evt := event.TypedUpdateEvent[*corev1.ConfigMap]{
				ObjectOld: oldCM,
				ObjectNew: cm,
			}
			handler.Update(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			item, _ := queue.Get()
			Expect(item.Name).To(Equal("test-cm"))
			Expect(item.Namespace).To(Equal("test-ns"))
		})

		It("should enqueue update event with only old object", func() {
			evt := event.TypedUpdateEvent[*corev1.ConfigMap]{
				ObjectOld: cm,
				ObjectNew: nil,
			}
			handler.Update(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			item, _ := queue.Get()
			Expect(item.Name).To(Equal("test-cm"))
			Expect(item.Namespace).To(Equal("test-ns"))
		})

		It("should not enqueue update event with no objects", func() {
			evt := event.TypedUpdateEvent[*corev1.ConfigMap]{
				ObjectOld: nil,
				ObjectNew: nil,
			}
			handler.Update(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(0))
		})

		It("should use custom priority function when provided", func() {
			customPriorityCallCount := 0
			handler.GetPriority = func(
				ctx context.Context,
				eventType kubeutil.EventType,
				newObj, oldObj *corev1.ConfigMap,
				defaultPriority int) int {
				customPriorityCallCount++
				Expect(eventType).To(Equal(kubeutil.EventUpdate))
				return defaultPriority + 200
			}

			oldCM := cm.DeepCopy()
			oldCM.ResourceVersion = "0"

			evt := event.TypedUpdateEvent[*corev1.ConfigMap]{
				ObjectOld: oldCM,
				ObjectNew: cm,
			}
			handler.Update(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			// Custom priority function is only called for priority queues
			Expect(customPriorityCallCount).To(Equal(0))
		})

		It("should handle same resource version", func() {
			oldCM := cm.DeepCopy()

			evt := event.TypedUpdateEvent[*corev1.ConfigMap]{
				ObjectOld: oldCM,
				ObjectNew: cm,
			}
			handler.Update(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
		})
	})

	Context("Delete", func() {
		It("should enqueue delete event with valid object", func() {
			evt := event.TypedDeleteEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Delete(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			item, _ := queue.Get()
			Expect(item.Name).To(Equal("test-cm"))
			Expect(item.Namespace).To(Equal("test-ns"))
		})

		It("should not enqueue delete event with nil object", func() {
			evt := event.TypedDeleteEvent[*corev1.ConfigMap]{
				Object: nil,
			}
			handler.Delete(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(0))
		})

		It("should use custom priority function when provided", func() {
			customPriorityCallCount := 0
			handler.GetPriority = func(
				ctx context.Context,
				eventType kubeutil.EventType,
				newObj, oldObj *corev1.ConfigMap,
				defaultPriority int) int {
				customPriorityCallCount++
				Expect(eventType).To(Equal(kubeutil.EventDelete))
				return defaultPriority + 300
			}

			evt := event.TypedDeleteEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Delete(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			// Custom priority function is only called for priority queues
			Expect(customPriorityCallCount).To(Equal(0))
		})
	})

	Context("Generic", func() {
		It("should enqueue generic event with valid object", func() {
			evt := event.TypedGenericEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Generic(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			item, _ := queue.Get()
			Expect(item.Name).To(Equal("test-cm"))
			Expect(item.Namespace).To(Equal("test-ns"))
		})

		It("should not enqueue generic event with nil object", func() {
			evt := event.TypedGenericEvent[*corev1.ConfigMap]{
				Object: nil,
			}
			handler.Generic(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(0))
		})

		It("should use custom priority function when provided", func() {
			customPriorityCallCount := 0
			handler.GetPriority = func(
				ctx context.Context,
				eventType kubeutil.EventType,
				newObj, oldObj *corev1.ConfigMap,
				defaultPriority int) int {
				customPriorityCallCount++
				Expect(eventType).To(Equal(kubeutil.EventGeneric))
				return defaultPriority + 400
			}

			evt := event.TypedGenericEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Generic(ctx, evt, queue)

			Expect(queue.Len()).To(Equal(1))
			// Custom priority function is only called for priority queues
			Expect(customPriorityCallCount).To(Equal(0))
		})
	})

	Context("Priority constants", func() {
		It("should have defined priority constants", func() {
			// Just verify the constants exist and are in expected order
			Expect(kubeutil.PriorityCreate).To(BeNumerically(">", kubeutil.PriorityDelete))
			Expect(kubeutil.PriorityDelete).To(BeNumerically(">", kubeutil.PriorityUpdate))
			Expect(kubeutil.PriorityUpdate).To(BeNumerically(">", kubeutil.PriorityGeneric))
		})
	})
})

var _ = Describe("TypedEnqueueRequestForObject with PriorityQueue", func() {
	var (
		ctx           context.Context
		handler       *kubeutil.TypedEnqueueRequestForObject[*corev1.ConfigMap]
		priorityQueue priorityqueue.PriorityQueue[reconcile.Request]
		cm            *corev1.ConfigMap
	)

	BeforeEach(func() {
		ctx = log.IntoContext(context.Background(), log.Log)
		handler = &kubeutil.TypedEnqueueRequestForObject[*corev1.ConfigMap]{
			Logger: log.Log,
		}
		priorityQueue = priorityqueue.New[reconcile.Request]("test-queue")
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "test-cm",
				Namespace:       "test-ns",
				ResourceVersion: "1",
			},
		}
	})

	AfterEach(func() {
		if priorityQueue != nil {
			priorityQueue.ShutDown()
		}
	})

	Context("Create with PriorityQueue", func() {
		It("should enqueue create event with default priority when IsInInitialList is true", func() {
			evt := event.TypedCreateEvent[*corev1.ConfigMap]{
				Object:          cm,
				IsInInitialList: true,
			}
			handler.Create(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			item, _ := priorityQueue.Get()
			Expect(item.Name).To(Equal("test-cm"))
			Expect(item.Namespace).To(Equal("test-ns"))
		})

		It("should enqueue create event with default priority when GetPriority is nil", func() {
			evt := event.TypedCreateEvent[*corev1.ConfigMap]{
				Object:          cm,
				IsInInitialList: false,
			}
			handler.Create(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			item, _ := priorityQueue.Get()
			Expect(item.Name).To(Equal("test-cm"))
		})

		It("should use custom priority function when provided and not in initial list", func() {
			customPriorityCallCount := 0
			handler.GetPriority = func(
				ctx context.Context,
				eventType kubeutil.EventType,
				newObj, oldObj *corev1.ConfigMap,
				defaultPriority int) int {
				customPriorityCallCount++
				Expect(eventType).To(Equal(kubeutil.EventCreate))
				Expect(newObj).To(Equal(cm))
				Expect(oldObj).To(Equal(cm))
				Expect(defaultPriority).To(Equal(kubeutil.PriorityCreate))
				return defaultPriority + 100
			}

			evt := event.TypedCreateEvent[*corev1.ConfigMap]{
				Object:          cm,
				IsInInitialList: false,
			}
			handler.Create(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			Expect(customPriorityCallCount).To(Equal(1))
		})
	})

	Context("Update with PriorityQueue", func() {
		It("should enqueue update event with default priority when resource versions match", func() {
			oldCM := cm.DeepCopy()

			evt := event.TypedUpdateEvent[*corev1.ConfigMap]{
				ObjectOld: oldCM,
				ObjectNew: cm,
			}
			handler.Update(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			item, _ := priorityQueue.Get()
			Expect(item.Name).To(Equal("test-cm"))
		})

		It("should enqueue update event with custom priority when resource versions differ", func() {
			customPriorityCallCount := 0
			handler.GetPriority = func(
				ctx context.Context,
				eventType kubeutil.EventType,
				newObj, oldObj *corev1.ConfigMap,
				defaultPriority int) int {
				customPriorityCallCount++
				Expect(eventType).To(Equal(kubeutil.EventUpdate))
				Expect(newObj).To(Equal(cm))
				Expect(oldObj.GetResourceVersion()).To(Equal("0"))
				Expect(defaultPriority).To(Equal(kubeutil.PriorityUpdate))
				return defaultPriority + 200
			}

			oldCM := cm.DeepCopy()
			oldCM.ResourceVersion = "0"

			evt := event.TypedUpdateEvent[*corev1.ConfigMap]{
				ObjectOld: oldCM,
				ObjectNew: cm,
			}
			handler.Update(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			Expect(customPriorityCallCount).To(Equal(1))
		})

		It("should enqueue update event with GetPriority nil and different resource versions", func() {
			oldCM := cm.DeepCopy()
			oldCM.ResourceVersion = "0"

			evt := event.TypedUpdateEvent[*corev1.ConfigMap]{
				ObjectOld: oldCM,
				ObjectNew: cm,
			}
			handler.Update(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
		})

		// Note: The case where only ObjectOld exists but ObjectNew is nil
		// cannot be tested with a priority queue because addToQueueUpdate
		// assumes both objects are non-nil when checking resource versions.
		// This is covered in the regular queue tests above.
	})

	Context("Delete with PriorityQueue", func() {
		It("should enqueue delete event with default priority when GetPriority is nil", func() {
			evt := event.TypedDeleteEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Delete(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			item, _ := priorityQueue.Get()
			Expect(item.Name).To(Equal("test-cm"))
		})

		It("should use custom priority function when provided", func() {
			customPriorityCallCount := 0
			handler.GetPriority = func(
				ctx context.Context,
				eventType kubeutil.EventType,
				newObj, oldObj *corev1.ConfigMap,
				defaultPriority int) int {
				customPriorityCallCount++
				Expect(eventType).To(Equal(kubeutil.EventDelete))
				Expect(newObj).To(Equal(cm))
				Expect(oldObj).To(Equal(cm))
				Expect(defaultPriority).To(Equal(kubeutil.PriorityDelete))
				return defaultPriority + 300
			}

			evt := event.TypedDeleteEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Delete(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			Expect(customPriorityCallCount).To(Equal(1))
		})
	})

	Context("Generic with PriorityQueue", func() {
		It("should enqueue generic event with default priority when GetPriority is nil", func() {
			evt := event.TypedGenericEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Generic(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			item, _ := priorityQueue.Get()
			Expect(item.Name).To(Equal("test-cm"))
		})

		It("should use custom priority function when provided", func() {
			customPriorityCallCount := 0
			handler.GetPriority = func(
				ctx context.Context,
				eventType kubeutil.EventType,
				newObj, oldObj *corev1.ConfigMap,
				defaultPriority int) int {
				customPriorityCallCount++
				Expect(eventType).To(Equal(kubeutil.EventGeneric))
				Expect(newObj).To(Equal(cm))
				Expect(oldObj).To(Equal(cm))
				Expect(defaultPriority).To(Equal(kubeutil.PriorityGeneric))
				return defaultPriority + 400
			}

			evt := event.TypedGenericEvent[*corev1.ConfigMap]{
				Object: cm,
			}
			handler.Generic(ctx, evt, priorityQueue)

			Expect(priorityQueue.Len()).To(Equal(1))
			Expect(customPriorityCallCount).To(Equal(1))
		})
	})
})
