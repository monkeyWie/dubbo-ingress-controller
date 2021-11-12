package watcher

import (
	"context"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"fmt"
	v1beta12 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"

	"github.com/bep/debounce"
	"time"
)

type Watcher struct {
	client       kubernetes.Interface
	eventHandler EventHandler
}

func NewWatcher(client kubernetes.Interface, eventHandler EventHandler) *Watcher {
	return &Watcher{
		client:       client,
		eventHandler: eventHandler,
	}
}

func (w *Watcher) Start(ctx context.Context) error {
	factory := informers.NewSharedInformerFactory(w.client, time.Minute)
	ingressLister := factory.Extensions().V1beta1().Ingresses().Lister()

	handleAdd := func(obj interface{}) {
		ingress, ok := obj.(*v1beta12.Ingress)
		if ok {
			eventData := ingressToEvent(ingress)
			if eventData != nil {
				w.eventHandler.OnAdd(eventData)
			}
		}
	}

	handleUpdate := func(oldObj, newObj interface{}) {
		ingressNew, ok := newObj.(*v1beta12.Ingress)
		if ok {
			eventDataNew := ingressToEvent(ingressNew)
			if eventDataNew != nil {
				w.eventHandler.OnUpdate(ingressToEvent(oldObj.(*v1beta12.Ingress)), eventDataNew)
			}
		}
	}

	handleDelete := func(obj interface{}) {
		ingress, ok := obj.(*v1beta12.Ingress)
		if ok {
			eventData := ingressToEvent(ingress)
			if eventData != nil {
				w.eventHandler.OnDelete(eventData)
			}
		}
	}

	debounced := debounce.New(time.Second)
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			debounced(func() {
				handleAdd(obj)
			})
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			debounced(func() {
				handleUpdate(oldObj, newObj)
			})
		},
		DeleteFunc: func(obj interface{}) {
			debounced(func() {
				handleDelete(obj)
			})
		},
	}

	// 首次启动直接全量来一次
	w.fullSync(ingressLister)

	informer := factory.Extensions().V1beta1().Ingresses().Informer()
	informer.AddEventHandler(handler)
	informer.Run(ctx.Done())

	return nil
}

func (w *Watcher) fullSync(ingressLister v1beta1.IngressLister) {
	ingresses, err := ingressLister.List(labels.Everything())
	if err != nil {
		logger.Errorf("list ingress fail: %v", err)
		return
	}

	for _, ingress := range ingresses {
		ingressClass := ingress.Annotations["kubernetes.io/ingress.class"]
		if ingressClass == "dubbo" && ingress.Spec.Rules != nil {
			for _, rule := range ingress.Spec.Rules {
				if len(rule.HTTP.Paths) > 0 {
					backend := rule.HTTP.Paths[0].Backend
					eventData := &EventData{
						Host:    rule.Host,
						Service: fmt.Sprintf("%s:%d", backend.ServiceName, backend.ServicePort.IntVal),
					}
					w.eventHandler.OnAdd(eventData)
				}
			}
		}
	}
}

func ingressToEvent(ingress *v1beta12.Ingress) *EventData {
	ingressClass := ingress.Annotations["kubernetes.io/ingress.class"]
	if ingressClass == "dubbo" && len(ingress.Spec.Rules) > 0 {
		rule := ingress.Spec.Rules[0]
		if len(rule.HTTP.Paths) > 0 {
			backend := rule.HTTP.Paths[0].Backend
			return &EventData{
				Host:    rule.Host,
				Service: fmt.Sprintf("%s:%d", backend.ServiceName, backend.ServicePort.IntVal),
			}
		}
	}
	return nil
}
