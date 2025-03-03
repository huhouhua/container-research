package main

import (
	"flag"
	"fmt"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"time"
)

// 这两个先定义
type Controller struct {
	indexer  cache.Indexer
	queue    workqueue.TypedRateLimitingInterface[string]
	informer cache.Controller
}

func NewController(queue workqueue.TypedRateLimitingInterface[string], indexer cache.Indexer, informer cache.Controller) *Controller {
	return &Controller{
		informer: informer,
		indexer:  indexer,
		queue:    queue,
	}
}
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

	// 创建速率限制队列
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]())

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// 生成 key
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	})

	controller := NewController(queue, deploymentinformer.Informer().GetIndexer(), informer)

	stopper := make(chan struct{})
	defer close(stopper)

	// 启动 informer，List & Watch
	informerFactory.Start(stopper)
	informerFactory.WaitForCacheSync(stopper)

	// 处理队列中的事件
	go func() {
		for {
			if !controller.processNextItem() {
				break
			}
		}
	}()

	<-stopper
}

// 处理下一个
func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	err := c.syncToStdout(key)
	c.handleErr(err, key)
	return true
}

// 输出日志
func (c *Controller) syncToStdout(key string) error {
	// 通过 key 从 indexer 中获取完整的对象
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		fmt.Printf("Fetching object with key %s from store failed with %v\n", key, err)
		return err
	}

	if !exists {
		fmt.Printf("Deployment %s does not exist anymore\n", key)
	} else {
		deployment := obj.(*v1.Deployment)
		fmt.Printf("Sync/Add/Update for Deployment %s, Replicas: %d\n", deployment.Name, *deployment.Spec.Replicas)
		if deployment.Name == "test-deployment" {
			time.Sleep(2 * time.Second)
			return fmt.Errorf("simulated error for deployment %s", deployment.Name)
		}
	}
	return nil
}

// 错误处理
func (c *Controller) handleErr(err error, key string) {
	if err == nil {
		c.queue.Forget(key)
		return
	}

	if c.queue.NumRequeues(key) < 5 {
		fmt.Printf("Retry %d for key %s\n", c.queue.NumRequeues(key), key)
		// 重新加入队列，并且进行速率限制，这会让他过一段时间才会被处理，避免过度重试
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	fmt.Printf("Dropping deployment %q out of the queue: %v\n", key, err)
}
