package arena

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ChallengeSpec defines the structure of a challenge specification
type ChallengeSpec struct {
	Registry   *RegistrySpec   `yaml:"registry,omitempty"`
	ConfigMaps []ConfigMapSpec `yaml:"configMaps,omitempty"`
	Pods       []PodSpec       `yaml:"pods"`
	Services   []ServiceSpec   `yaml:"services,omitempty"`
}

// RegistrySpec defines private registry configuration
type RegistrySpec struct {
	CredentialsSecret string `yaml:"credentialsSecret"`
	URL               string `yaml:"url,omitempty"`
}

// ConfigMapSpec defines a ConfigMap in the challenge
type ConfigMapSpec struct {
	Name   string            `yaml:"name"`
	Data   map[string]string `yaml:"data"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

// VolumeSpec defines a volume in the challenge
type VolumeSpec struct {
	Name      string `yaml:"name"`
	ConfigMap string `yaml:"configMap,omitempty"`
}

// PodSpec defines a pod in the challenge
type PodSpec struct {
	Name      string            `yaml:"name"`
	Image     string            `yaml:"image"`
	Ports     []PortSpec        `yaml:"ports,omitempty"`
	Env       []EnvVar          `yaml:"env,omitempty"`
	EnvFrom   []string          `yaml:"envFrom,omitempty"`
	Volumes   []VolumeSpec      `yaml:"volumes,omitempty"`
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

// K8sManager abstracts Kubernetes operations for arena management.
type K8sManager interface {
	CreateNamespace(ctx context.Context, name string, labels map[string]string) error
	DeleteNamespace(ctx context.Context, name string) error
	CreateConfigMaps(ctx context.Context, namespace string, configMaps []*v1.ConfigMap) error
	DeleteConfigMaps(ctx context.Context, namespace string, configMapNames []string) error
	CreatePods(ctx context.Context, namespace string, pods []*v1.Pod) error
	DeletePods(ctx context.Context, namespace string, podNames []string) error
	CreateServices(ctx context.Context, namespace string, services []*v1.Service) error
	DeleteServices(ctx context.Context, namespace string, serviceNames []string) error
	ApplyNetworkPolicy(ctx context.Context, namespace string, vpnIP string, serviceIPs []string) error
	ApplyResourceQuota(ctx context.Context, namespace string, quota *ResourceUsage) error
	GetNamespaceStatus(ctx context.Context, name string) (*NamespaceStatus, error)
	ValidateSpec(ctx context.Context, challengeSpec string) error
	CreateResourcesFromSpec(ctx context.Context, namespace, challengeSpec string) ([]*ServiceEndpoint, error)

	// CIDR configuration methods
	GetClusterCIDR() string
	GetServiceCIDR() string
}

// k8sManager is a real implementation of the K8sManager interface.
type k8sManager struct {
	clientset   *k8sclient.Clientset
	clusterCIDR string // Cluster pod network CIDR
	serviceCIDR string // Kubernetes service network CIDR
}

func NewK8sManager(clusterCIDR, serviceCIDR string) K8sManager {
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

	return &k8sManager{
		clientset:   clientset,
		clusterCIDR: clusterCIDR,
		serviceCIDR: serviceCIDR,
	}
}

// GetClusterCIDR returns the cluster pod network CIDR.
func (k *k8sManager) GetClusterCIDR() string {
	return k.clusterCIDR
}

// GetServiceCIDR returns the Kubernetes service network CIDR.
func (k *k8sManager) GetServiceCIDR() string {
	return k.serviceCIDR
}

func (k *k8sManager) CreateNamespace(ctx context.Context, name string, labels map[string]string) error {
	log.Printf("📦 Creating Kubernetes namespace: %s with labels: %v", name, labels)
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	_, err := k.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		log.Printf("❌ Failed to create namespace %s: %v", name, err)
		return err
	}
	log.Printf("✅ Namespace created successfully: %s", name)
	return nil
}

func (k *k8sManager) DeleteNamespace(ctx context.Context, name string) error {
	log.Printf("🗑️ Deleting Kubernetes namespace: %s", name)
	err := k.clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		log.Printf("❌ Failed to delete namespace %s: %v", name, err)
		return err
	}
	log.Printf("✅ Namespace deletion initiated: %s", name)
	return nil
}

func (k *k8sManager) CreateConfigMaps(ctx context.Context, namespace string, configMaps []*v1.ConfigMap) error {
	for _, cm := range configMaps {
		_, err := k.clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create config map %s: %w", cm.Name, err)
		}
	}
	return nil
}

func (k *k8sManager) DeleteConfigMaps(ctx context.Context, namespace string, configMapNames []string) error {
	for _, name := range configMapNames {
		err := k.clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete config map %s: %w", name, err)
		}
	}
	return nil
}

func (k *k8sManager) CreatePods(ctx context.Context, namespace string, pods []*v1.Pod) error {
	for _, pod := range pods {
		_, err := k.clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create pod %s: %w", pod.Name, err)
		}
	}
	return nil
}

func (k *k8sManager) DeletePods(ctx context.Context, namespace string, podNames []string) error {
	for _, name := range podNames {
		err := k.clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete pod %s: %w", name, err)
		}
	}
	return nil
}

func (k *k8sManager) CreateServices(ctx context.Context, namespace string, services []*v1.Service) error {
	for _, svc := range services {
		_, err := k.clientset.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create service %s: %w", svc.Name, err)
		}
	}
	return nil
}

func (k *k8sManager) DeleteServices(ctx context.Context, namespace string, serviceNames []string) error {
	for _, name := range serviceNames {
		err := k.clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to delete service %s: %w", name, err)
		}
	}
	return nil
}

func (k *k8sManager) ApplyNetworkPolicy(ctx context.Context, namespace string, vpnIP string, serviceIPs []string) error {
	// Parse the VPN IP to ensure it's a valid CIDR
	_, ipnet, err := net.ParseCIDR(vpnIP)
	if err != nil {
		return fmt.Errorf("invalid VPN IP CIDR: %s", vpnIP)
	}

	// Use the parsed CIDR string
	vpnCidr := ipnet.String()
	clusterCidr := k.clusterCIDR // Use configurable cluster CIDR

	// Build ingress and egress rules with specific service IPs
	var ingressPeers []networkingv1.NetworkPolicyPeer
	var egressPeers []networkingv1.NetworkPolicyPeer

	// Add VPN and cluster CIDRs
	ingressPeers = append(ingressPeers, networkingv1.NetworkPolicyPeer{
		IPBlock: &networkingv1.IPBlock{CIDR: vpnCidr},
	})
	ingressPeers = append(ingressPeers, networkingv1.NetworkPolicyPeer{
		IPBlock: &networkingv1.IPBlock{CIDR: clusterCidr},
	})

	egressPeers = append(egressPeers, networkingv1.NetworkPolicyPeer{
		IPBlock: &networkingv1.IPBlock{CIDR: vpnCidr},
	})
	egressPeers = append(egressPeers, networkingv1.NetworkPolicyPeer{
		IPBlock: &networkingv1.IPBlock{CIDR: clusterCidr},
	})

	// Add specific service IPs
	for _, serviceIP := range serviceIPs {
		ingressPeers = append(ingressPeers, networkingv1.NetworkPolicyPeer{
			IPBlock: &networkingv1.IPBlock{CIDR: serviceIP},
		})
		egressPeers = append(egressPeers, networkingv1.NetworkPolicyPeer{
			IPBlock: &networkingv1.IPBlock{CIDR: serviceIP},
		})
	}

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
					From: ingressPeers,
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: egressPeers,
				},
			},
		},
	}
	_, err = k.clientset.NetworkingV1().NetworkPolicies(namespace).Create(ctx, policy, metav1.CreateOptions{})
	return err
}

func (k *k8sManager) ApplyResourceQuota(ctx context.Context, namespace string, quota *ResourceUsage) error {
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
	_, err := k.clientset.CoreV1().ResourceQuotas(namespace).Create(ctx, resourceQuota, metav1.CreateOptions{})
	return err
}

func (k *k8sManager) GetNamespaceStatus(ctx context.Context, name string) (*NamespaceStatus, error) {
	ns, err := k.clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	pods, err := k.clientset.CoreV1().Pods(name).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	services, err := k.clientset.CoreV1().Services(name).List(ctx, metav1.ListOptions{})
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
		endpoints, err := k.clientset.CoreV1().Endpoints(name).Get(ctx, svc.Name, metav1.GetOptions{})
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
	policies, err := k.clientset.NetworkingV1().NetworkPolicies(name).List(ctx, metav1.ListOptions{})
	if err == nil && len(policies.Items) > 0 {
		networkPolicyApplied = true
	}

	// Get resource quota
	var resourceUsage *ResourceUsage
	quotas, err := k.clientset.CoreV1().ResourceQuotas(name).List(ctx, metav1.ListOptions{})
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

func (k *k8sManager) ValidateSpec(ctx context.Context, challengeSpec string) error {
	// Basic validation: must be valid YAML and contain at least a 'pods' list
	var spec map[string]interface{}
	if err := yaml.Unmarshal([]byte(challengeSpec), &spec); err != nil {
		return fmt.Errorf("spec is not valid YAML: %w", err)
	}

	// Check that 'pods' field exists
	podsField, exists := spec["pods"]
	if !exists {
		return fmt.Errorf("spec must contain a 'pods' field")
	}

	// Check that 'pods' field is a list
	if _, ok := podsField.([]interface{}); !ok {
		return fmt.Errorf("'pods' field must be a list")
	}

	return nil
}

func (k *k8sManager) CreateResourcesFromSpec(ctx context.Context, namespace, challengeSpec string) ([]*ServiceEndpoint, error) {
	log.Printf("🚀 Creating resources from spec in namespace: %s", namespace)

	// Parse the challenge spec
	var spec ChallengeSpec
	if err := yaml.Unmarshal([]byte(challengeSpec), &spec); err != nil {
		log.Printf("❌ Failed to parse challenge spec: %v", err)
		return nil, fmt.Errorf("failed to parse challenge spec: %w", err)
	}
	log.Printf("✅ Challenge spec parsed - ConfigMaps: %d, Pods: %d, Services: %d", len(spec.ConfigMaps), len(spec.Pods), len(spec.Services))

	// Validate registry configuration if needed
	if spec.Registry != nil {
		if err := k.validateRegistryCredentials("control-plane", spec.Registry.CredentialsSecret); err != nil {
			log.Printf("❌ Registry credentials validation failed: %v", err)
			return nil, fmt.Errorf("registry credentials validation failed: %w", err)
		}

		// Copy registry secret to arena namespace
		log.Printf("🔐 Copying registry secret to arena namespace: %s", namespace)
		if err := k.copyRegistrySecret("control-plane", namespace, spec.Registry.CredentialsSecret); err != nil {
			log.Printf("❌ Failed to copy registry secret: %v", err)
			return nil, fmt.Errorf("failed to copy registry secret: %w", err)
		}
		log.Printf("✅ Registry secret copied to arena namespace")
	}

	// Create config maps first
	var configMaps []*v1.ConfigMap
	for _, cmSpec := range spec.ConfigMaps {
		log.Printf("📦 Creating config map: %s", cmSpec.Name)
		cm, err := k.createConfigMapFromSpec(namespace, cmSpec)
		if err != nil {
			log.Printf("❌ Failed to create config map %s: %v", cmSpec.Name, err)
			return nil, fmt.Errorf("failed to create config map %s: %w", cmSpec.Name, err)
		}
		configMaps = append(configMaps, cm)
		log.Printf("✅ Config map spec created: %s", cmSpec.Name)
	}

	// Create the config maps
	if len(configMaps) > 0 {
		log.Printf("🚀 Creating %d config maps in namespace: %s", len(configMaps), namespace)
		if err := k.CreateConfigMaps(ctx, namespace, configMaps); err != nil {
			log.Printf("❌ Failed to create config maps: %v", err)
			return nil, fmt.Errorf("failed to create config maps: %w", err)
		}
		log.Printf("✅ All config maps created successfully")
	}

	// Create pods first
	var pods []*v1.Pod
	for _, podSpec := range spec.Pods {
		log.Printf("📦 Creating pod: %s with image: %s", podSpec.Name, podSpec.Image)
		pod, err := k.createPodFromSpec(namespace, podSpec, spec.Registry)
		if err != nil {
			log.Printf("❌ Failed to create pod %s: %v", podSpec.Name, err)
			return nil, fmt.Errorf("failed to create pod %s: %w", podSpec.Name, err)
		}
		pods = append(pods, pod)
		log.Printf("✅ Pod spec created: %s", podSpec.Name)
	}

	// Create the pods
	log.Printf("🚀 Creating %d pods in namespace: %s", len(pods), namespace)
	if err := k.CreatePods(ctx, namespace, pods); err != nil {
		log.Printf("❌ Failed to create pods: %v", err)
		return nil, fmt.Errorf("failed to create pods: %w", err)
	}
	log.Printf("✅ All pods created successfully")

	// Create services
	var services []*v1.Service
	for _, svcSpec := range spec.Services {
		log.Printf("🔗 Creating service: %s", svcSpec.Name)
		svc, err := k.createServiceFromSpec(namespace, svcSpec)
		if err != nil {
			log.Printf("❌ Failed to create service %s: %v", svcSpec.Name, err)
			return nil, fmt.Errorf("failed to create service %s: %w", svcSpec.Name, err)
		}
		services = append(services, svc)
		log.Printf("✅ Service spec created: %s", svcSpec.Name)
	}

	// Create the services
	if len(services) > 0 {
		log.Printf("🚀 Creating %d services in namespace: %s", len(services), namespace)
		if err := k.CreateServices(ctx, namespace, services); err != nil {
			log.Printf("❌ Failed to create services: %v", err)
			return nil, fmt.Errorf("failed to create services: %w", err)
		}
		log.Printf("✅ All services created successfully")
	}

	// Now fetch the created services to get their assigned cluster IPs
	var endpoints []*ServiceEndpoint
	for _, svcSpec := range spec.Services {
		log.Printf("🔍 Fetching service details: %s", svcSpec.Name)
		createdSvc, err := k.clientset.CoreV1().Services(namespace).Get(ctx, svcSpec.Name, metav1.GetOptions{})
		if err != nil {
			log.Printf("❌ Failed to get created service %s: %v", svcSpec.Name, err)
			return nil, fmt.Errorf("failed to get created service %s: %w", svcSpec.Name, err)
		}

		// Create endpoint info with the actual cluster IP
		endpoint := &ServiceEndpoint{
			Name:      svcSpec.Name,
			ClusterIp: createdSvc.Spec.ClusterIP,
			Ports:     make([]int32, len(svcSpec.Ports)),
		}
		for i, port := range svcSpec.Ports {
			endpoint.Ports[i] = port.Port
		}
		endpoints = append(endpoints, endpoint)
		log.Printf("✅ Service endpoint created: %s -> %s", svcSpec.Name, createdSvc.Spec.ClusterIP)
	}

	log.Printf("🎉 Resource creation completed - Endpoints: %d", len(endpoints))
	return endpoints, nil
}

func (k *k8sManager) createConfigMapFromSpec(namespace string, spec ConfigMapSpec) (*v1.ConfigMap, error) {
	// Set default labels if none provided
	labels := spec.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["app"] = spec.Name

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: spec.Data,
	}

	return cm, nil
}

func (k *k8sManager) createPodFromSpec(namespace string, spec PodSpec, registry *RegistrySpec) (*v1.Pod, error) {
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

	// Convert envFrom ConfigMap references
	var envFromSources []v1.EnvFromSource
	for _, configMapName := range spec.EnvFrom {
		envFromSources = append(envFromSources, v1.EnvFromSource{
			ConfigMapRef: &v1.ConfigMapEnvSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: configMapName,
				},
			},
		})
	}

	// Convert volume specs
	var volumes []v1.Volume
	var volumeMounts []v1.VolumeMount
	for _, volSpec := range spec.Volumes {
		if volSpec.ConfigMap != "" {
			// Create volume for ConfigMap
			volumes = append(volumes, v1.Volume{
				Name: volSpec.Name,
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{
							Name: volSpec.ConfigMap,
						},
					},
				},
			})

			// Create volume mount with convention: /config/<configmap-name>
			volumeMounts = append(volumeMounts, v1.VolumeMount{
				Name:      volSpec.Name,
				MountPath: fmt.Sprintf("/config/%s", volSpec.ConfigMap),
				ReadOnly:  true,
			})
		}
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

	// Determine image and pull policy
	imageToUse := spec.Image
	imagePullPolicy := v1.PullIfNotPresent
	var imagePullSecrets []v1.LocalObjectReference

	if registry != nil {
		// Check if this image should be treated as private
		if k.isPrivateRegistryImage(spec.Image, registry.URL) {
			// Normalize the image for the private registry
			imageToUse = k.normalizeImage(spec.Image, registry.URL)
			imagePullSecrets = []v1.LocalObjectReference{
				{Name: registry.CredentialsSecret},
			}
			imagePullPolicy = v1.PullIfNotPresent
		}
		// If not private, use the original image as-is
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.PodSpec{
			ImagePullSecrets: imagePullSecrets,
			Containers: []v1.Container{
				{
					Name:            spec.Name,
					Image:           imageToUse,
					ImagePullPolicy: imagePullPolicy,
					Ports:           containerPorts,
					Env:             envVars,
					EnvFrom:         envFromSources,
					VolumeMounts:    volumeMounts,
					Resources:       resourceRequirements,
				},
			},
			Volumes:       volumes,
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
			Name:      spec.Name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.ServiceSpec{
			Type:     v1.ServiceType(serviceType),
			Ports:    servicePorts,
			Selector: spec.Selector,
		},
	}

	return svc, nil
}

// validateRegistryCredentials checks if the specified registry secret exists in the namespace
func (k *k8sManager) validateRegistryCredentials(namespace, secretName string) error {
	log.Printf("🔐 Validating registry credentials secret: %s in namespace: %s", secretName, namespace)
	
	_, err := k.clientset.CoreV1().Secrets(namespace).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("registry credentials secret %s not found in namespace %s", secretName, namespace)
		}
		return fmt.Errorf("failed to get registry credentials secret %s: %w", secretName, err)
	}
	
	log.Printf("✅ Registry credentials secret validated: %s", secretName)
	return nil
}

// copyRegistrySecret copies a registry secret from source namespace to target namespace
func (k *k8sManager) copyRegistrySecret(sourceNamespace, targetNamespace, secretName string) error {
	log.Printf("🔐 Copying registry secret %s from %s to %s", secretName, sourceNamespace, targetNamespace)
	
	// Get the secret from source namespace
	sourceSecret, err := k.clientset.CoreV1().Secrets(sourceNamespace).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get source secret %s from namespace %s: %w", secretName, sourceNamespace, err)
	}
	
	// Create a new secret object for the target namespace
	targetSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: targetNamespace,
		},
		Type: sourceSecret.Type,
		Data: sourceSecret.Data,
	}
	
	// Create the secret in target namespace
	_, err = k.clientset.CoreV1().Secrets(targetNamespace).Create(context.Background(), targetSecret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			log.Printf("✅ Registry secret already exists in target namespace: %s", targetNamespace)
			return nil
		}
		return fmt.Errorf("failed to create secret %s in namespace %s: %w", secretName, targetNamespace, err)
	}
	
	log.Printf("✅ Registry secret copied successfully to namespace: %s", targetNamespace)
	return nil
}



// isPrivateRegistryImage determines if an image is from a private registry
func (k *k8sManager) isPrivateRegistryImage(image, registryURL string) bool {
	// If no registry URL is specified, assume all images are private
	if registryURL == "" {
		return true
	}
	
	// Check if the image belongs to this registry
	// If image already has the registry URL prefix, it's from this registry
	if strings.HasPrefix(image, registryURL+"/") {
		return true
	}
	
    // If image has a different registry host (contains . or :), it's not from this registry
	if strings.Contains(image, "/") {
		parts := strings.SplitN(image, "/", 2)
		firstPart := parts[0]
		if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
			return false
		}
	}
	
    // For simple or relative image names (no explicit registry host),
    // treat as private when a registry URL is configured. They will be
    // normalized to the configured registry and require imagePullSecrets.
    return true
}

// isDockerHubImage determines if an image is from Docker Hub
func (k *k8sManager) isDockerHubImage(image string) bool {
	// Docker Hub images are typically simple names or have docker.io prefix
	// But since we're using a private registry, we'll be conservative
	return false
}

// normalizeImage converts relative image paths to full paths with registry URL
func (k *k8sManager) normalizeImage(image, registryURL string) string {
	if registryURL == "" {
		return image
	}
	
	// If image already has the registry URL prefix, don't modify it
	if strings.HasPrefix(image, registryURL+"/") {
		return image
	}
	
	// If image has a different registry host (contains . or :), don't normalize
	if strings.Contains(image, "/") {
		parts := strings.SplitN(image, "/", 2)
		firstPart := parts[0]
		if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
			return image
		}
	}
	
	// Normalize relative image path by prepending registry URL
	return registryURL + "/" + image
}


