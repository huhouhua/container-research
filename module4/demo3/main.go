package main

import (
	"flag"
	"fmt"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"time"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "/root/study/container-research/module1/demo1/module/k3s/config.yaml", "this is the kubeconfig file path")
	cfg, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		fmt.Printf("err %s", err.Error())
	}
	clientSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Printf("err %s", err.Error())
	}
	informerFactory := informers.NewSharedInformerFactory(clientSet, time.Hour*12)
	deploymentinformer := informerFactory.Apps().V1().Deployments()
	informer := deploymentinformer.Informer()
	lister := deploymentinformer.Lister()

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			item := obj.(*v1.Deployment)

			fmt.Println("add ", item.Name, item.Spec.Template.Spec.Containers[0].Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldo := oldObj.(*v1.Deployment)
			newo := newObj.(*v1.Deployment)
			fmt.Println(fmt.Sprintf("update %s %s", oldo.Name, newo.Name))
		},
		DeleteFunc: func(obj interface{}) {
			deployment := obj.(*v1.Deployment)
			fmt.Println("delete ", deployment.Name)
		},
	})

	stopper := make(chan struct{})
	defer close(stopper)

	// 启动 informer，List & Watch
	informerFactory.Start(stopper)
	// 等待所有启动的 Informer 的缓存被同步
	informerFactory.WaitForCacheSync(stopper)

	// Lister，从本地缓存中获取 default 中的所有 deployment 列表，最终从 Indexer 取数据
	deployments, err := lister.Deployments("default").List(labels.Everything())
	if err != nil {
		panic(err)
	}
	for idx, deploy := range deployments {
		fmt.Printf("%d -> %s\n", idx+1, deploy.Name)
	}
	// 阻塞主 goroutine
	<-stopper
}
