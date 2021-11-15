package watcher

import (
	"context"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"fmt"
	v1beta12 "k8s.io/api/extensions/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
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
	w.fullSync(w.client)

	informer := factory.Extensions().V1beta1().Ingresses().Informer()
	informer.AddEventHandler(handler)
	informer.Run(ctx.Done())

	return nil
}

func (w *Watcher) fullSync(client kubernetes.Interface) {
	ingressList, err := client.ExtensionsV1beta1().Ingresses("").List(v1.ListOptions{})
	if err != nil {
		logger.Errorf("list controller fail: %v", err)
		return
	}

	for _, ingress := range ingressList.Items {
		eventData := ingressToEvent(&ingress)
		if eventData != nil {
			w.eventHandler.OnAdd(eventData)
		}
	}
}

func ingressToEvent(ingress *v1beta12.Ingress) *EventData {
	// 过滤出dubbo ingress
	// TODO dubbo现在是写死的，后续需要改成可配置的
	ingressClass := ingress.Annotations["kubernetes.io/ingress.class"]
	if ingressClass == "dubbo" && len(ingress.Spec.Rules) > 0 {
		rule := ingress.Spec.Rules[0]
		if len(rule.HTTP.Paths) > 0 {
			backend := rule.HTTP.Paths[0].Backend
			return &EventData{
				Host:    rule.Host,
				Service: fmt.Sprintf("%s:%d", backend.ServiceName+"."+ingress.Namespace, backend.ServicePort.IntVal),
			}
		}
	}
	return nil
}
