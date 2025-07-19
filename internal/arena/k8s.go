package arena

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ChallengeSpec defines the structure of a challenge specification
type ChallengeSpec struct {
	Pods     []PodSpec     `yaml:"pods"`
	Services []ServiceSpec `yaml:"services,omitempty"`
}

// PodSpec defines a pod in the challenge
type PodSpec struct {
	Name      string            `yaml:"name"`
	Image     string            `yaml:"image"`
	Ports     []PortSpec        `yaml:"ports,omitempty"`
	Env       []EnvVar          `yaml:"env,omitempty"`
	Resources ResourceSpec      `yaml:"resources,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

// ServiceSpec defines a service in the challenge
type ServiceSpec struct {
	Name     string            `yaml:"name"`
	Type     string            `yaml:"type,omitempty"`
	Ports    []PortSpec        `yaml:"ports"`
	Selector map[string]string `yaml:"selector"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}

// PortSpec defines a port mapping
type PortSpec struct {
	Name       string `yaml:"name,omitempty"`
	Port       int32  `yaml:"port"`
	TargetPort int32  `yaml:"targetPort,omitempty"`
	Protocol   string `yaml:"protocol,omitempty"`
}

// EnvVar defines an environment variable
type EnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// ResourceSpec defines resource requests and limits
type ResourceSpec struct {
	Requests map[string]string `yaml:"requests,omitempty"`
	Limits   map[string]string `yaml:"limits,omitempty"`
}

// K8sManager handles all Kubernetes resource management for user arenas.
type K8sManager interface {
	CreateNamespace(name string, labels map[string]string) error
	DeleteNamespace(name string) error
	CreatePods(namespace string, pods []*v1.Pod) error
	DeletePods(namespace string, podNames []string) error
	CreateServices(namespace string, services []*v1.Service) error
	DeleteServices(namespace string, serviceNames []string) error
	ApplyNetworkPolicy(namespace string, vpnIP string) error
	ApplyResourceQuota(namespace string, quota *ResourceUsage) error
	GetNamespaceStatus(name string) (*NamespaceStatus, error)
	ValidateSpec(challengeSpec string) error
	CreateResourcesFromSpec(namespace, challengeSpec string) ([]*ServiceEndpoint, error)
}

// k8sManager is a real implementation of the K8sManager interface.
type k8sManager struct {
	clientset *k8sclient.Clientset
}

func NewK8sManager() K8sManager {
	// Try in-cluster config first, then fall back to kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Printf("Failed to get in-cluster config, trying kubeconfig: %v", err)
		config, err = clientcmd.BuildConfigFromFlags("", "~/.kube/config")
		if err != nil {
			log.Fatalf("Failed to get kubeconfig: %v", err)
		}
	}

	clientset, err := k8sclient.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create clientset: %v", err)
	}

	return &k8sManager{clientset: clientset}
}

func (k *k8sManager) CreateNamespace(name string, labels map[string]string) error {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	_, err := k.clientset.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	return err
}

func (k *k8sManager) DeleteNamespace(name string) error {
	return k.clientset.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{})
}

func (k *k8sManager) CreatePods(namespace string, pods []*v1.Pod) error {
	for _, pod := range pods {
		_, err := k.clientset.CoreV1().Pods(namespace).Create(context.Background(), pod, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create pod %s: %w", pod.Name, err)
		}
	}
	return nil
}

func (k *k8sManager) DeletePods(namespace string, podNames []string) error {
	for _, name := range podNames {
		err := k.clientset.CoreV1().Pods(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete pod %s: %w", name, err)
		}
	}
	return nil
}

func (k *k8sManager) CreateServices(namespace string, services []*v1.Service) error {
	for _, svc := range services {
		_, err := k.clientset.CoreV1().Services(namespace).Create(context.Background(), svc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create service %s: %w", svc.Name, err)
		}
	}
	return nil
}

func (k *k8sManager) DeleteServices(namespace string, serviceNames []string) error {
	for _, name := range serviceNames {
		err := k.clientset.CoreV1().Services(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete service %s: %w", name, err)
		}
	}
	return nil
}

func (k *k8sManager) ApplyNetworkPolicy(namespace string, vpnIP string) error {
	// Parse the VPN IP to ensure it's a valid CIDR
	_, ipnet, err := net.ParseCIDR(vpnIP)
	if err != nil {
		return fmt.Errorf("invalid VPN IP CIDR: %s", vpnIP)
	}
	
	// Use the parsed CIDR string
	cidr := ipnet.String()
	
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "arena-vpn-policy",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: cidr,
							},
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: cidr,
							},
						},
					},
				},
			},
		},
	}
	_, err = k.clientset.NetworkingV1().NetworkPolicies(namespace).Create(context.Background(), policy, metav1.CreateOptions{})
	return err
}

func (k *k8sManager) ApplyResourceQuota(namespace string, quota *ResourceUsage) error {
	resourceQuota := &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "arena-quota",
			Namespace: namespace,
		},
		Spec: v1.ResourceQuotaSpec{
			Hard: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(quota.Cpu),
				v1.ResourceMemory: resource.MustParse(quota.Memory),
				v1.ResourcePods:   resource.MustParse(quota.Pods),
			},
		},
	}
	_, err := k.clientset.CoreV1().ResourceQuotas(namespace).Create(context.Background(), resourceQuota, metav1.CreateOptions{})
	return err
}

func (k *k8sManager) GetNamespaceStatus(name string) (*NamespaceStatus, error) {
	ns, err := k.clientset.CoreV1().Namespaces().Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	pods, err := k.clientset.CoreV1().Pods(name).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	services, err := k.clientset.CoreV1().Services(name).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	// Count pod statuses
	podCount := len(pods.Items)
	var podsRunning, podsPending, podsFailed, podsSucceeded, podsUnknown, servicesReady int32
	podStatuses := make([]*PodStatus, 0, podCount)

	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case v1.PodRunning:
			podsRunning++
		case v1.PodPending:
			podsPending++
		case v1.PodFailed:
			podsFailed++
		case v1.PodSucceeded:
			podsSucceeded++
		default:
			podsUnknown++
		}

		// Calculate age
		age := "unknown"
		if !pod.CreationTimestamp.IsZero() {
			age = time.Since(pod.CreationTimestamp.Time).Round(time.Minute).String()
		}

		// Check if pod is ready
		ready := false
		for _, condition := range pod.Status.Conditions {
			if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
				ready = true
				break
			}
		}

		restarts := int32(0)
		if len(pod.Status.ContainerStatuses) > 0 {
			restarts = pod.Status.ContainerStatuses[0].RestartCount
		}

		podStatuses = append(podStatuses, &PodStatus{
			Name:     pod.Name,
			Phase:    string(pod.Status.Phase),
			Ready:    ready,
			Restarts: restarts,
			Age:      age,
			Ip:       pod.Status.PodIP,
			HostIp:   pod.Status.HostIP,
		})
	}

	// Count service statuses
	serviceCount := len(services.Items)
	serviceStatuses := make([]*ServiceStatus, 0, serviceCount)

	for _, svc := range services.Items {
		endpoints, err := k.clientset.CoreV1().Endpoints(name).Get(context.Background(), svc.Name, metav1.GetOptions{})
		ready := err == nil && len(endpoints.Subsets) > 0 && len(endpoints.Subsets[0].Addresses) > 0
		if ready {
			servicesReady++
		}

		ports := make([]int32, len(svc.Spec.Ports))
		for i, port := range svc.Spec.Ports {
			ports[i] = port.Port
		}

		endpointCount := 0
		if endpoints != nil && len(endpoints.Subsets) > 0 {
			endpointCount = len(endpoints.Subsets[0].Addresses)
		}

		serviceStatuses = append(serviceStatuses, &ServiceStatus{
			Name:      svc.Name,
			Type:      string(svc.Spec.Type),
			ClusterIp: svc.Spec.ClusterIP,
			Ports:     ports,
			Endpoints: int32(endpointCount),
			Ready:     ready,
		})
	}

	// Check if network policy exists
	networkPolicyApplied := false
	policies, err := k.clientset.NetworkingV1().NetworkPolicies(name).List(context.Background(), metav1.ListOptions{})
	if err == nil && len(policies.Items) > 0 {
		networkPolicyApplied = true
	}

	// Get resource quota
	var resourceUsage *ResourceUsage
	quotas, err := k.clientset.CoreV1().ResourceQuotas(name).List(context.Background(), metav1.ListOptions{})
	if err == nil && len(quotas.Items) > 0 {
		quota := quotas.Items[0]
		resourceUsage = &ResourceUsage{
			Cpu:    quota.Spec.Hard.Cpu().String(),
			Memory: quota.Spec.Hard.Memory().String(),
			Pods:   quota.Spec.Hard.Pods().String(),
		}
	}

	return &NamespaceStatus{
		Name:                 ns.Name,
		Phase:                string(ns.Status.Phase),
		Labels:               ns.Labels,
		PodCount:             int32(podCount),
		ServiceCount:         int32(serviceCount),
		PodsRunning:          int32(podsRunning),
		PodsPending:          int32(podsPending),
		PodsFailed:           int32(podsFailed),
		PodsSucceeded:        int32(podsSucceeded),
		PodsUnknown:          int32(podsUnknown),
		ServicesReady:        int32(servicesReady),
		ServicesTotal:        int32(serviceCount),
		NetworkPolicyApplied: networkPolicyApplied,
		ResourceUsage:        resourceUsage,
		Pods:                 podStatuses,
		Services:             serviceStatuses,
	}, nil
}

func (k *k8sManager) ValidateSpec(challengeSpec string) error {
	// Basic validation: must be valid YAML and contain at least a 'pods' list
	var spec map[string]interface{}
	if err := yaml.Unmarshal([]byte(challengeSpec), &spec); err != nil {
		return fmt.Errorf("spec is not valid YAML: %w", err)
	}
	pods, ok := spec["pods"]
	if !ok {
		return fmt.Errorf("spec must contain a 'pods' field")
	}
	if _, ok := pods.([]interface{}); !ok {
		return fmt.Errorf("'pods' field must be a list")
	}
	return nil
}

func (k *k8sManager) CreateResourcesFromSpec(namespace, challengeSpec string) ([]*ServiceEndpoint, error) {
	// Parse the challenge spec
	var spec ChallengeSpec
	if err := yaml.Unmarshal([]byte(challengeSpec), &spec); err != nil {
		return nil, fmt.Errorf("failed to parse challenge spec: %w", err)
	}

	// Create pods first
	var pods []*v1.Pod
	for _, podSpec := range spec.Pods {
		pod, err := k.createPodFromSpec(namespace, podSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to create pod %s: %w", podSpec.Name, err)
		}
		pods = append(pods, pod)
	}

	// Create the pods
	if err := k.CreatePods(namespace, pods); err != nil {
		return nil, fmt.Errorf("failed to create pods: %w", err)
	}

	// Create services
	var services []*v1.Service
	var endpoints []*ServiceEndpoint
	for _, svcSpec := range spec.Services {
		svc, err := k.createServiceFromSpec(namespace, svcSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to create service %s: %w", svcSpec.Name, err)
		}
		services = append(services, svc)

		// Create endpoint info
		endpoint := &ServiceEndpoint{
			Name:      svcSpec.Name,
			ClusterIp: svc.Spec.ClusterIP,
			Ports:     make([]int32, len(svcSpec.Ports)),
		}
		for i, port := range svcSpec.Ports {
			endpoint.Ports[i] = port.Port
		}
		endpoints = append(endpoints, endpoint)
	}

	// Create the services
	if err := k.CreateServices(namespace, services); err != nil {
		return nil, fmt.Errorf("failed to create services: %w", err)
	}

	return endpoints, nil
}

func (k *k8sManager) createPodFromSpec(namespace string, spec PodSpec) (*v1.Pod, error) {
	// Convert port specs to container ports
	var containerPorts []v1.ContainerPort
	for _, port := range spec.Ports {
		containerPort := v1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.TargetPort,
			Protocol:      v1.Protocol(port.Protocol),
		}
		if port.TargetPort == 0 {
			containerPort.ContainerPort = port.Port
		}
		containerPorts = append(containerPorts, containerPort)
	}

	// Convert environment variables
	var envVars []v1.EnvVar
	for _, env := range spec.Env {
		envVars = append(envVars, v1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
	}

	// Convert resource specs
	var resourceRequirements v1.ResourceRequirements
	if spec.Resources.Requests != nil || spec.Resources.Limits != nil {
		if spec.Resources.Requests != nil {
			resourceRequirements.Requests = make(v1.ResourceList)
			for key, value := range spec.Resources.Requests {
				resourceRequirements.Requests[v1.ResourceName(key)] = resource.MustParse(value)
			}
		}
		if spec.Resources.Limits != nil {
			resourceRequirements.Limits = make(v1.ResourceList)
			for key, value := range spec.Resources.Limits {
				resourceRequirements.Limits[v1.ResourceName(key)] = resource.MustParse(value)
			}
		}
	}

	// Set default labels if none provided
	labels := spec.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["app"] = spec.Name

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   spec.Name,
			Labels: labels,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:      spec.Name,
					Image:     spec.Image,
					Ports:     containerPorts,
					Env:       envVars,
					Resources: resourceRequirements,
				},
			},
			RestartPolicy: v1.RestartPolicyAlways,
		},
	}

	return pod, nil
}

func (k *k8sManager) createServiceFromSpec(namespace string, spec ServiceSpec) (*v1.Service, error) {
	// Convert port specs to service ports
	var servicePorts []v1.ServicePort
	for _, port := range spec.Ports {
		servicePort := v1.ServicePort{
			Name:       port.Name,
			Port:       port.Port,
			TargetPort: intstr.FromInt(int(port.TargetPort)),
			Protocol:   v1.Protocol(port.Protocol),
		}
		if port.TargetPort == 0 {
			servicePort.TargetPort = intstr.FromInt(int(port.Port))
		}
		servicePorts = append(servicePorts, servicePort)
	}

	// Set default service type if not specified
	serviceType := spec.Type
	if serviceType == "" {
		serviceType = string(v1.ServiceTypeClusterIP)
	}

	// Set default labels if none provided
	labels := spec.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["app"] = spec.Name

	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   spec.Name,
			Labels: labels,
		},
		Spec: v1.ServiceSpec{
			Type:     v1.ServiceType(serviceType),
			Ports:    servicePorts,
			Selector: spec.Selector,
		},
	}

	return svc, nil
}
