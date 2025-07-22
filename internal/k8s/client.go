package k8s

import (
	"context"
	"fmt"
	"log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetAMFPods retrieves the AMF pods in the specified namespace.
func GetAMFPods(clientset *kubernetes.Clientset, namespace string) ([]v1.Pod, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "nf=amf",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list AMF pods: %v", err)
	}
	return pods.Items, nil
}

// GetAMFServices retrieves the AMF services in the specified namespace.
func GetAMFServices(clientset *kubernetes.Clientset, namespace string) ([]v1.Service, error) {
	services, err := clientset.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "nf=amf",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list AMF services: %v", err)
	}
	return services.Items, nil
}