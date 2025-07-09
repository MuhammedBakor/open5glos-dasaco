package watcher

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/hasukiHT/5glos/internal/balancer"
	"github.com/hasukiHT/5glos/internal/config"
)

// K8sWatcher monitors Kubernetes for AMF pod changes
type K8sWatcher struct {
	clientset *kubernetes.Clientset
	cfg       config.KubernetesConfig
	logger    *zap.Logger
}

// AMFPool manages the pool of AMF instances
type AMFPool struct {
	balancer balancer.LoadBalancer
	logger   *zap.Logger
}

// New creates a new Kubernetes watcher
func New(cfg config.KubernetesConfig, logger *zap.Logger) (*K8sWatcher, error) {
	var config *rest.Config
	var err error

	if cfg.InCluster {
		// Create in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
	} else {
		// Use kubeconfig file
		kubeconfigPath := cfg.KubeconfigPath
		if kubeconfigPath == "" {
			kubeconfigPath = clientcmd.RecommendedHomeFile
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
		}
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &K8sWatcher{
		clientset: clientset,
		cfg:       cfg,
		logger:    logger,
	}, nil
}

// StartAMFWatcher starts watching for AMF pods and returns an AMFPool
func (w *K8sWatcher) StartAMFWatcher(ctx context.Context) (*AMFPool, error) {
	// Create load balancer (this would be passed from main)
	// For now, we'll create a simple round-robin balancer
	lb := balancer.NewRoundRobinBalancer(nil, w.logger.Named("balancer"))

	pool := &AMFPool{
		balancer: lb,
		logger:   w.logger,
	}

	// Initial discovery of existing AMF pods
	if err := w.discoverExistingAMFs(pool); err != nil {
		w.logger.Error("Failed to discover existing AMFs", zap.Error(err))
	}

	// Start watching for changes
	go w.watchAMFPods(ctx, pool)

	return pool, nil
}

// discoverExistingAMFs discovers AMF pods that are already running
func (w *K8sWatcher) discoverExistingAMFs(pool *AMFPool) error {
	labelSelector := w.cfg.AMFSelector

	pods, err := w.clientset.CoreV1().Pods(w.cfg.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list AMF pods: %w", err)
	}

	w.logger.Info("Discovered existing AMF pods",
		zap.Int("count", len(pods.Items)),
		zap.String("namespace", w.cfg.Namespace),
		zap.String("selector", labelSelector),
	)

	for _, pod := range pods.Items {
		if w.isPodReady(&pod) {
			amf := w.podToAMFInstance(&pod)
			if amf != nil {
				pool.balancer.AddAMF(amf)
				w.logger.Info("Added existing AMF",
					zap.String("amf_id", amf.ID),
					zap.String("address", amf.GetAddress()),
				)
			}
		}
	}

	return nil
}

// watchAMFPods watches for AMF pod changes
func (w *K8sWatcher) watchAMFPods(ctx context.Context, pool *AMFPool) {
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Stopping AMF pod watcher")
			return
		default:
			if err := w.watchPods(ctx, pool); err != nil {
				w.logger.Error("Pod watcher error", zap.Error(err))
				// Wait before retrying
				time.Sleep(5 * time.Second)
			}
		}
	}
}

// watchPods performs the actual pod watching
func (w *K8sWatcher) watchPods(ctx context.Context, pool *AMFPool) error {
	labelSelector := w.cfg.AMFSelector

	watcher, err := w.clientset.CoreV1().Pods(w.cfg.Namespace).Watch(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to create pod watcher: %w", err)
	}
	defer watcher.Stop()

	w.logger.Info("Started watching AMF pods",
		zap.String("namespace", w.cfg.Namespace),
		zap.String("selector", labelSelector),
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("pod watcher channel closed")
			}

			w.handlePodEvent(event, pool)
		}
	}
}

// handlePodEvent handles pod watch events
func (w *K8sWatcher) handlePodEvent(event watch.Event, pool *AMFPool) {
	pod, ok := event.Object.(*corev1.Pod)
	if !ok {
		w.logger.Warn("Received non-pod object in pod watcher")
		return
	}

	podID := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

	switch event.Type {
	case watch.Added:
		w.logger.Info("AMF pod added", zap.String("pod", podID))
		if w.isPodReady(pod) {
			amf := w.podToAMFInstance(pod)
			if amf != nil {
				pool.balancer.AddAMF(amf)
				w.logger.Info("Added new AMF",
					zap.String("amf_id", amf.ID),
					zap.String("address", amf.GetAddress()),
				)
			}
		}

	case watch.Modified:
		w.logger.Debug("AMF pod modified", zap.String("pod", podID))
		if w.isPodReady(pod) {
			amf := w.podToAMFInstance(pod)
			if amf != nil {
				pool.balancer.UpdateAMF(amf)
				w.logger.Debug("Updated AMF",
					zap.String("amf_id", amf.ID),
					zap.String("address", amf.GetAddress()),
				)
			}
		} else {
			// Pod is no longer ready, mark as unhealthy
			amfID := w.podToAMFID(pod)
			if existingAMF, exists := pool.balancer.GetAMF(amfID); exists {
				existingAMF.SetHealthy(false)
				pool.balancer.UpdateAMF(existingAMF)
				w.logger.Warn("AMF marked as unhealthy",
					zap.String("amf_id", amfID),
					zap.String("pod", podID),
				)
			}
		}

	case watch.Deleted:
		w.logger.Info("AMF pod deleted", zap.String("pod", podID))
		amfID := w.podToAMFID(pod)
		pool.balancer.RemoveAMF(amfID)
		w.logger.Info("Removed AMF",
			zap.String("amf_id", amfID),
			zap.String("pod", podID),
		)

	case watch.Error:
		w.logger.Error("Pod watch error", zap.Any("object", event.Object))
	}
}

// isPodReady checks if a pod is ready
func (w *K8sWatcher) isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

// podToAMFInstance converts a pod to an AMF instance
func (w *K8sWatcher) podToAMFInstance(pod *corev1.Pod) *balancer.AMFInstance {
	if pod.Status.PodIP == "" {
		w.logger.Warn("Pod has no IP address", zap.String("pod", pod.Name))
		return nil
	}

	amfID := w.podToAMFID(pod)
	port := w.getAMFPort(pod)

	return &balancer.AMFInstance{
		ID:      amfID,
		Address: pod.Status.PodIP,
		Port:    port,
		Healthy: w.isPodReady(pod),
		Weight:  1, // Default weight
	}
}

// podToAMFID generates an AMF ID from a pod
func (w *K8sWatcher) podToAMFID(pod *corev1.Pod) string {
	return fmt.Sprintf("%s-%s", pod.Namespace, pod.Name)
}

// getAMFPort extracts the AMF port from pod annotations or uses default
func (w *K8sWatcher) getAMFPort(pod *corev1.Pod) int {
	// Try to get port from annotations
	if portStr, exists := pod.Annotations["amf.port"]; exists {
		if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
			return port
		}
	}

	// Try to get port from container ports
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == "ngap" || port.Name == "amf" {
				return int(port.ContainerPort)
			}
		}
	}

	// Default AMF port
	return 38412
}

// GetBalancer returns the load balancer
func (p *AMFPool) GetBalancer() balancer.LoadBalancer {
	return p.balancer
}

// SelectAMF selects an AMF for a UE
func (p *AMFPool) SelectAMF(ueID string) (*balancer.AMFInstance, error) {
	return p.balancer.SelectAMF(ueID)
}

// GetStats returns pool statistics
func (p *AMFPool) GetStats() balancer.BalancerStats {
	return p.balancer.GetStats()
}
