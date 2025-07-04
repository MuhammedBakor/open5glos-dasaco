/*
Package kube provides Kubernetes client functionality for managing resources.
*/
package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hasukiHT/5glos/amf"
	"github.com/hasukiHT/5glos/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// PodEventType represents the type of pod event
type PodEventType string

const (
	PodEventAdded    PodEventType = "ADDED"
	PodEventModified PodEventType = "MODIFIED"
	PodEventDeleted  PodEventType = "DELETED"
)

// PodEvent represents a pod change event
type PodEvent struct {
	Type     PodEventType
	Endpoint *amf.AMFEndpoint
}

// Client wraps the Kubernetes client
type Client struct {
	clientset *kubernetes.Clientset
}

// NewClient creates a new Kubernetes client
func NewClient() (*Client, error) {
	config, err := getKubeConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kube config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &Client{clientset: clientset}, nil
}

// getKubeConfig gets the Kubernetes configuration
func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to local kubeconfig
		kubeconfig := filepath.Join(homeDir(), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}
	}
	return config, nil
}

// homeDir returns the home directory
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // Windows
}

// ListAMFPods lists AMF pods in the specified namespace with label selector
func (c *Client) ListAMFPods(namespace, labelSelector string) ([]*amf.AMFEndpoint, error) {
	pods, err := c.clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var endpoints []*amf.AMFEndpoint
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Status.PodIP == "" {
			continue
		}

		endpoint := podToAMFEndpoint(&pod)
		if endpoint != nil {
			endpoints = append(endpoints, endpoint)
		}
	}

	return endpoints, nil
}

// WatchAMFPods watches for changes to AMF pods
func (c *Client) WatchAMFPods(ctx context.Context, namespace, labelSelector string) (<-chan PodEvent, error) {
	watcher, err := c.clientset.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	eventChan := make(chan PodEvent, 10)

	go func() {
		defer close(eventChan)
		defer watcher.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.ResultChan():
				if !ok {
					utils.LogWarn("Watcher channel closed, restarting...")
					// Try to restart the watcher
					time.Sleep(5 * time.Second)
					newWatcher, err := c.clientset.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
						LabelSelector: labelSelector,
					})
					if err != nil {
						utils.LogError("Failed to restart watcher", err)
						return
					}
					watcher = newWatcher
					continue
				}

				pod, ok := event.Object.(*corev1.Pod)
				if !ok {
					continue
				}

				var eventType PodEventType
				switch event.Type {
				case watch.Added:
					eventType = PodEventAdded
				case watch.Modified:
					eventType = PodEventModified
				case watch.Deleted:
					eventType = PodEventDeleted
				default:
					continue
				}

				endpoint := podToAMFEndpoint(pod)
				if endpoint != nil {
					select {
					case eventChan <- PodEvent{Type: eventType, Endpoint: endpoint}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return eventChan, nil
}

// podToAMFEndpoint converts a Kubernetes pod to an AMF endpoint
func podToAMFEndpoint(pod *corev1.Pod) *amf.AMFEndpoint {
	if pod.Status.PodIP == "" {
		return nil
	}

	// Find SCTP port (default 38412 for NGAP)
	port := 38412
	for _, container := range pod.Spec.Containers {
		for _, portSpec := range container.Ports {
			if portSpec.Protocol == corev1.ProtocolSCTP {
				port = int(portSpec.ContainerPort)
				break
			}
		}
	}

	status := amf.StatusUnhealthy
	if pod.Status.Phase == corev1.PodRunning {
		// Check if all containers are ready
		allReady := true
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
				allReady = false
				break
			}
		}
		if allReady {
			status = amf.StatusHealthy
		}
	}

	return &amf.AMFEndpoint{
		ID:       pod.Name,
		IP:       pod.Status.PodIP,
		Port:     port,
		LastSeen: time.Now(),
		Status:   status,
	}
}
