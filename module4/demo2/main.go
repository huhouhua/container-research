package main

import (
	"context"
	"flag"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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
	timeout := int64(60 * 3)
	watcher, err := clientSet.CoreV1().Pods("default").Watch(context.TODO(), metav1.ListOptions{
		TimeoutSeconds: &timeout,
	})

	for event := range watcher.ResultChan() {
		item := event.Object.(*corev1.Pod)
		switch event.Type {
		case watch.Added:
			fmt.Println("add ", item.Name)
		case watch.Modified:
			fmt.Println("update ", item.Name, item.Spec.Containers[0].Name)
		case watch.Deleted:
			fmt.Println("delete ", item.Name)
		}
	}
}
