package arena

import (
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
)

// mockK8sManagerForTesting is a test-specific implementation that doesn't require a real clientset
type mockK8sManagerForTesting struct {
	k8sManager
}

func (m *mockK8sManagerForTesting) CreateConfigMaps(namespace string, configMaps []*v1.ConfigMap) error {
	// Mock implementation - just return success
	return nil
}

func (m *mockK8sManagerForTesting) CreatePods(namespace string, pods []*v1.Pod) error {
	// Mock implementation - just return success
	return nil
}

func (m *mockK8sManagerForTesting) CreateServices(namespace string, services []*v1.Service) error {
	// Mock implementation - just return success
	return nil
}

func TestK8sManager_ValidateSpec(t *testing.T) {
	mgr := &k8sManager{}

	tests := []struct {
		name    string
		spec    string
		wantErr bool
		msg     string
	}{
		{
			name:    "valid spec",
			spec:    "pods:\n  - name: testpod\n    image: busybox",
			wantErr: false,
		},
		{
			name:    "invalid yaml",
			spec:    "pods: [",
			wantErr: true,
			msg:     "spec is not valid YAML",
		},
		{
			name:    "missing pods field",
			spec:    "services:\n  - name: svc",
			wantErr: true,
			msg:     "spec must contain a 'pods' field",
		},
		{
			name:    "pods not a list",
			spec:    "pods: notalist",
			wantErr: true,
			msg:     "'pods' field must be a list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.ValidateSpec(context.Background(), tt.spec)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && tt.msg != "" && err != nil && err.Error() != tt.msg && !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("expected error message to contain '%s', got '%v'", tt.msg, err)
			}
		})
	}
}

func TestK8sManager_ParseChallengeSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr bool
		check   func(*testing.T, *ChallengeSpec)
	}{
		{
			name: "valid spec with pod only",
			spec: `
pods:
  - name: testpod
    image: busybox
    ports:
      - port: 80
        targetPort: 8080
    env:
      - name: TEST_VAR
        value: test_value
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "200m"
        memory: "256Mi"
`,
			wantErr: false,
			check: func(t *testing.T, spec *ChallengeSpec) {
				if len(spec.Pods) != 1 {
					t.Errorf("expected 1 pod, got %d", len(spec.Pods))
				}
				if spec.Pods[0].Name != "testpod" {
					t.Errorf("expected pod name 'testpod', got '%s'", spec.Pods[0].Name)
				}
				if spec.Pods[0].Image != "busybox" {
					t.Errorf("expected image 'busybox', got '%s'", spec.Pods[0].Image)
				}
				if len(spec.Pods[0].Ports) != 1 {
					t.Errorf("expected 1 port, got %d", len(spec.Pods[0].Ports))
				}
				if spec.Pods[0].Ports[0].Port != 80 {
					t.Errorf("expected port 80, got %d", spec.Pods[0].Ports[0].Port)
				}
			},
		},
		{
			name: "valid spec with pod and service",
			spec: `
pods:
  - name: webapp
    image: nginx
    ports:
      - port: 80
        targetPort: 80
    labels:
      app: webapp
services:
  - name: webapp-service
    type: ClusterIP
    ports:
      - port: 80
        targetPort: 80
    selector:
      app: webapp
`,
			wantErr: false,
			check: func(t *testing.T, spec *ChallengeSpec) {
				if len(spec.Pods) != 1 {
					t.Errorf("expected 1 pod, got %d", len(spec.Pods))
				}
				if len(spec.Services) != 1 {
					t.Errorf("expected 1 service, got %d", len(spec.Services))
				}
				if spec.Services[0].Name != "webapp-service" {
					t.Errorf("expected service name 'webapp-service', got '%s'", spec.Services[0].Name)
				}
			},
		},
		{
			name: "invalid yaml",
			spec: `
pods:
  - name: testpod
    image: busybox
    ports: [
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var spec ChallengeSpec
			err := yaml.Unmarshal([]byte(tt.spec), &spec)

			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, &spec)
			}
		})
	}
}

func TestK8sManager_CreatePodFromSpec(t *testing.T) {
	mgr := &k8sManager{}

	tests := []struct {
		name    string
		spec    PodSpec
		wantErr bool
	}{
		{
			name: "basic pod",
			spec: PodSpec{
				Name:  "testpod",
				Image: "busybox",
			},
			wantErr: false,
		},
		{
			name: "pod with ports",
			spec: PodSpec{
				Name:  "webapp",
				Image: "nginx",
				Ports: []PortSpec{
					{Port: 80, TargetPort: 8080, Protocol: "TCP"},
					{Name: "https", Port: 443, TargetPort: 8443},
				},
			},
			wantErr: false,
		},
		{
			name: "pod with env vars",
			spec: PodSpec{
				Name:  "app",
				Image: "myapp:latest",
				Env: []EnvVar{
					{Name: "DB_HOST", Value: "localhost"},
					{Name: "DEBUG", Value: "true"},
				},
			},
			wantErr: false,
		},
		{
			name: "pod with resources",
			spec: PodSpec{
				Name:  "resource-pod",
				Image: "busybox",
				Resources: ResourceSpec{
					Requests: map[string]string{
						"cpu":    "100m",
						"memory": "128Mi",
					},
					Limits: map[string]string{
						"cpu":    "200m",
						"memory": "256Mi",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod, err := mgr.createPodFromSpec("test-namespace", tt.spec, nil)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && pod == nil {
				t.Error("expected pod, got nil")
			}
			if !tt.wantErr && pod != nil {
				if pod.Name != tt.spec.Name {
					t.Errorf("expected pod name %s, got %s", tt.spec.Name, pod.Name)
				}
				if len(pod.Spec.Containers) != 1 {
					t.Errorf("expected 1 container, got %d", len(pod.Spec.Containers))
				}
				if pod.Spec.Containers[0].Image != tt.spec.Image {
					t.Errorf("expected image %s, got %s", tt.spec.Image, pod.Spec.Containers[0].Image)
				}
			}
		})
	}
}

func TestK8sManager_CreateServiceFromSpec(t *testing.T) {
	mgr := &k8sManager{}

	tests := []struct {
		name    string
		spec    ServiceSpec
		wantErr bool
	}{
		{
			name: "basic service",
			spec: ServiceSpec{
				Name: "test-service",
				Ports: []PortSpec{
					{Port: 80, TargetPort: 8080},
				},
				Selector: map[string]string{"app": "test"},
			},
			wantErr: false,
		},
		{
			name: "service with multiple ports",
			spec: ServiceSpec{
				Name: "multi-port-service",
				Type: "ClusterIP",
				Ports: []PortSpec{
					{Name: "http", Port: 80, TargetPort: 8080},
					{Name: "https", Port: 443, TargetPort: 8443},
				},
				Selector: map[string]string{"app": "webapp"},
			},
			wantErr: false,
		},
		{
			name: "service with labels",
			spec: ServiceSpec{
				Name: "labeled-service",
				Ports: []PortSpec{
					{Port: 80, TargetPort: 80},
				},
				Selector: map[string]string{"app": "test"},
				Labels: map[string]string{
					"environment": "production",
					"tier":        "frontend",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := mgr.createServiceFromSpec("test-namespace", tt.spec)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && svc == nil {
				t.Error("expected service, got nil")
			}
			if !tt.wantErr && svc != nil {
				if svc.Name != tt.spec.Name {
					t.Errorf("expected service name %s, got %s", tt.spec.Name, svc.Name)
				}
				if len(svc.Spec.Ports) != len(tt.spec.Ports) {
					t.Errorf("expected %d ports, got %d", len(tt.spec.Ports), len(svc.Spec.Ports))
				}
			}
		})
	}
}

func TestK8sManager_CreateConfigMapFromSpec(t *testing.T) {
	mgr := &k8sManager{}

	tests := []struct {
		name    string
		spec    ConfigMapSpec
		wantErr bool
	}{
		{
			name: "basic configmap",
			spec: ConfigMapSpec{
				Name: "test-config",
				Data: map[string]string{
					"config.yaml": "key: value",
					"app.conf":    "debug=true",
				},
			},
			wantErr: false,
		},
		{
			name: "configmap with labels",
			spec: ConfigMapSpec{
				Name: "webapp-template",
				Data: map[string]string{
					"index.html": "<h1>{{ flag }}</h1>",
					"style.css":  "body { font-family: Inter; }",
				},
				Labels: map[string]string{
					"app":  "webapp",
					"type": "template",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm, err := mgr.createConfigMapFromSpec("test-namespace", tt.spec)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && cm == nil {
				t.Error("expected configmap, got nil")
			}
			if !tt.wantErr && cm != nil {
				if cm.Name != tt.spec.Name {
					t.Errorf("expected configmap name %s, got %s", tt.spec.Name, cm.Name)
				}
				if cm.Namespace != "test-namespace" {
					t.Errorf("expected namespace test-namespace, got %s", cm.Namespace)
				}
				if len(cm.Data) != len(tt.spec.Data) {
					t.Errorf("expected %d data entries, got %d", len(tt.spec.Data), len(cm.Data))
				}
				for key, value := range tt.spec.Data {
					if cm.Data[key] != value {
						t.Errorf("expected data[%s]=%s, got %s", key, value, cm.Data[key])
					}
				}
				// Check default app label is added
				if cm.Labels["app"] != tt.spec.Name {
					t.Errorf("expected app label %s, got %s", tt.spec.Name, cm.Labels["app"])
				}
			}
		})
	}
}

func TestK8sManager_CreatePodWithConfigMapSupport(t *testing.T) {
	mgr := &k8sManager{}

	tests := []struct {
		name    string
		spec    PodSpec
		wantErr bool
		check   func(*testing.T, *v1.Pod)
	}{
		{
			name: "pod with configmap volume",
			spec: PodSpec{
				Name:  "webapp",
				Image: "nginx",
				Volumes: []VolumeSpec{
					{Name: "config-vol", ConfigMap: "webapp-config"},
				},
			},
			wantErr: false,
			check: func(t *testing.T, pod *v1.Pod) {
				if len(pod.Spec.Volumes) != 1 {
					t.Errorf("expected 1 volume, got %d", len(pod.Spec.Volumes))
				}
				if pod.Spec.Volumes[0].Name != "config-vol" {
					t.Errorf("expected volume name config-vol, got %s", pod.Spec.Volumes[0].Name)
				}
				if pod.Spec.Volumes[0].VolumeSource.ConfigMap.Name != "webapp-config" {
					t.Errorf("expected configmap name webapp-config, got %s", pod.Spec.Volumes[0].VolumeSource.ConfigMap.Name)
				}
				if len(pod.Spec.Containers[0].VolumeMounts) != 1 {
					t.Errorf("expected 1 volume mount, got %d", len(pod.Spec.Containers[0].VolumeMounts))
				}
				expectedMountPath := "/config/webapp-config"
				if pod.Spec.Containers[0].VolumeMounts[0].MountPath != expectedMountPath {
					t.Errorf("expected mount path %s, got %s", expectedMountPath, pod.Spec.Containers[0].VolumeMounts[0].MountPath)
				}
			},
		},
		{
			name: "pod with envFrom configmap",
			spec: PodSpec{
				Name:    "app",
				Image:   "myapp:latest",
				EnvFrom: []string{"app-config", "secrets-config"},
			},
			wantErr: false,
			check: func(t *testing.T, pod *v1.Pod) {
				if len(pod.Spec.Containers[0].EnvFrom) != 2 {
					t.Errorf("expected 2 envFrom sources, got %d", len(pod.Spec.Containers[0].EnvFrom))
				}
				if pod.Spec.Containers[0].EnvFrom[0].ConfigMapRef.Name != "app-config" {
					t.Errorf("expected configmap name app-config, got %s", pod.Spec.Containers[0].EnvFrom[0].ConfigMapRef.Name)
				}
				if pod.Spec.Containers[0].EnvFrom[1].ConfigMapRef.Name != "secrets-config" {
					t.Errorf("expected configmap name secrets-config, got %s", pod.Spec.Containers[0].EnvFrom[1].ConfigMapRef.Name)
				}
			},
		},
		{
			name: "pod with both volume and envFrom",
			spec: PodSpec{
				Name:    "fullapp",
				Image:   "nginx",
				EnvFrom: []string{"app-env"},
				Volumes: []VolumeSpec{
					{Name: "templates", ConfigMap: "webapp-template"},
				},
			},
			wantErr: false,
			check: func(t *testing.T, pod *v1.Pod) {
				// Check envFrom
				if len(pod.Spec.Containers[0].EnvFrom) != 1 {
					t.Errorf("expected 1 envFrom source, got %d", len(pod.Spec.Containers[0].EnvFrom))
				}
				// Check volumes
				if len(pod.Spec.Volumes) != 1 {
					t.Errorf("expected 1 volume, got %d", len(pod.Spec.Volumes))
				}
				if len(pod.Spec.Containers[0].VolumeMounts) != 1 {
					t.Errorf("expected 1 volume mount, got %d", len(pod.Spec.Containers[0].VolumeMounts))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod, err := mgr.createPodFromSpec("test-namespace", tt.spec, nil)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && pod == nil {
				t.Error("expected pod, got nil")
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, pod)
			}
		})
	}
}

func TestK8sManager_CompleteConfigMapWorkflow(t *testing.T) {
	spec := `
configMaps:
  - name: webapp-template
    data:
      index.html: |
        <h1>{{ flag }}</h1>
        <p>Welcome to the challenge!</p>
      style.css: |
        body { font-family: Inter, sans-serif; }
    labels:
      app: webapp
      type: template
  - name: app-config
    data:
      DATABASE_URL: "postgresql://localhost:5432/db"
      DEBUG: "true"
      API_KEY: "secret123"

pods:
  - name: webapp
    image: nginx:alpine
    ports:
      - port: 80
        targetPort: 80
    envFrom:
      - app-config
    volumes:
      - name: templates
        configMap: webapp-template
    labels:
      app: webapp

services:
  - name: webapp-service
    type: ClusterIP
    ports:
      - port: 80
        targetPort: 80
    selector:
      app: webapp
`

	// Parse the YAML
	var challengeSpec ChallengeSpec
	err := yaml.Unmarshal([]byte(spec), &challengeSpec)
	if err != nil {
		t.Fatalf("failed to parse challenge spec: %v", err)
	}

	// Verify ConfigMaps
	if len(challengeSpec.ConfigMaps) != 2 {
		t.Errorf("expected 2 configmaps, got %d", len(challengeSpec.ConfigMaps))
	}

	// Check first ConfigMap
	webappTemplate := challengeSpec.ConfigMaps[0]
	if webappTemplate.Name != "webapp-template" {
		t.Errorf("expected configmap name webapp-template, got %s", webappTemplate.Name)
	}
	if len(webappTemplate.Data) != 2 {
		t.Errorf("expected 2 data entries, got %d", len(webappTemplate.Data))
	}
	if !strings.Contains(webappTemplate.Data["index.html"], "{{ flag }}") {
		t.Error("expected index.html to contain {{ flag }}")
	}
	if !strings.Contains(webappTemplate.Data["style.css"], "Inter") {
		t.Error("expected style.css to contain Inter font")
	}

	// Check second ConfigMap
	appConfig := challengeSpec.ConfigMaps[1]
	if appConfig.Name != "app-config" {
		t.Errorf("expected configmap name app-config, got %s", appConfig.Name)
	}
	if appConfig.Data["DEBUG"] != "true" {
		t.Errorf("expected DEBUG=true, got %s", appConfig.Data["DEBUG"])
	}

	// Verify Pod configuration
	if len(challengeSpec.Pods) != 1 {
		t.Errorf("expected 1 pod, got %d", len(challengeSpec.Pods))
	}

	pod := challengeSpec.Pods[0]
	if len(pod.EnvFrom) != 1 {
		t.Errorf("expected 1 envFrom, got %d", len(pod.EnvFrom))
	}
	if pod.EnvFrom[0] != "app-config" {
		t.Errorf("expected envFrom app-config, got %s", pod.EnvFrom[0])
	}
	if len(pod.Volumes) != 1 {
		t.Errorf("expected 1 volume, got %d", len(pod.Volumes))
	}
	if pod.Volumes[0].ConfigMap != "webapp-template" {
		t.Errorf("expected volume configmap webapp-template, got %s", pod.Volumes[0].ConfigMap)
	}

	// Test creating Kubernetes objects
	mgr := &k8sManager{}

	// Test ConfigMap creation
	for _, cmSpec := range challengeSpec.ConfigMaps {
		cm, err := mgr.createConfigMapFromSpec("test-namespace", cmSpec)
		if err != nil {
			t.Errorf("failed to create configmap %s: %v", cmSpec.Name, err)
		}
		if cm.Name != cmSpec.Name {
			t.Errorf("expected configmap name %s, got %s", cmSpec.Name, cm.Name)
		}
	}

	// Test Pod creation with ConfigMap references
	for _, podSpec := range challengeSpec.Pods {
		pod, err := mgr.createPodFromSpec("test-namespace", podSpec, nil)
		if err != nil {
			t.Errorf("failed to create pod %s: %v", podSpec.Name, err)
		}
		
		// Verify envFrom is correctly configured
		if len(pod.Spec.Containers[0].EnvFrom) != len(podSpec.EnvFrom) {
			t.Errorf("expected %d envFrom sources, got %d", len(podSpec.EnvFrom), len(pod.Spec.Containers[0].EnvFrom))
		}
		
		// Verify volumes are correctly configured
		if len(pod.Spec.Volumes) != len(podSpec.Volumes) {
			t.Errorf("expected %d volumes, got %d", len(podSpec.Volumes), len(pod.Spec.Volumes))
		}
		
		// Verify volume mounts are correctly configured
		if len(pod.Spec.Containers[0].VolumeMounts) != len(podSpec.Volumes) {
			t.Errorf("expected %d volume mounts, got %d", len(podSpec.Volumes), len(pod.Spec.Containers[0].VolumeMounts))
		}
		
		// Check mount path convention
		if len(pod.Spec.Containers[0].VolumeMounts) > 0 {
			expectedPath := "/config/webapp-template"
			actualPath := pod.Spec.Containers[0].VolumeMounts[0].MountPath
			if actualPath != expectedPath {
				t.Errorf("expected mount path %s, got %s", expectedPath, actualPath)
			}
		}
	}

	// Test Service creation (should work the same as before)
	for _, svcSpec := range challengeSpec.Services {
		svc, err := mgr.createServiceFromSpec("test-namespace", svcSpec)
		if err != nil {
			t.Errorf("failed to create service %s: %v", svcSpec.Name, err)
		}
		if svc.Name != svcSpec.Name {
			t.Errorf("expected service name %s, got %s", svcSpec.Name, svc.Name)
		}
	}
}

func TestK8sManager_PrivateRegistrySupport(t *testing.T) {
	mgr := &k8sManager{}

	tests := []struct {
		name     string
		spec     PodSpec
		registry *RegistrySpec
		wantErr  bool
		check    func(*testing.T, *v1.Pod, *RegistrySpec)
	}{
		{
			name: "pod with private registry image",
			spec: PodSpec{
				Name:  "private-app",
				Image: "registry.example.com/myorg/myapp:v1.2.3",
			},
			registry: &RegistrySpec{
				CredentialsSecret: "my-registry-creds",
				URL:               "registry.example.com",
			},
			wantErr: false,
			check: func(t *testing.T, pod *v1.Pod, registry *RegistrySpec) {
				// Should have imagePullSecrets
				if len(pod.Spec.ImagePullSecrets) != 1 {
					t.Errorf("expected 1 imagePullSecret, got %d", len(pod.Spec.ImagePullSecrets))
				}
				if pod.Spec.ImagePullSecrets[0].Name != registry.CredentialsSecret {
					t.Errorf("expected imagePullSecret %s, got %s", registry.CredentialsSecret, pod.Spec.ImagePullSecrets[0].Name)
				}
				// Should use IfNotPresent policy (since images are pre-warmed)
				if pod.Spec.Containers[0].ImagePullPolicy != v1.PullIfNotPresent {
					t.Errorf("expected ImagePullPolicy IfNotPresent, got %v", pod.Spec.Containers[0].ImagePullPolicy)
				}
				// Image should remain normalized
				expectedImage := "registry.example.com/myorg/myapp:v1.2.3"
				if pod.Spec.Containers[0].Image != expectedImage {
					t.Errorf("expected image %s, got %s", expectedImage, pod.Spec.Containers[0].Image)
				}
			},
		},
		{
			name: "pod with relative private registry image",
			spec: PodSpec{
				Name:  "relative-app",
				Image: "myorg/myapp:latest",
			},
			registry: &RegistrySpec{
				CredentialsSecret: "my-registry-creds",
				URL:               "registry.example.com",
			},
			wantErr: false,
			check: func(t *testing.T, pod *v1.Pod, registry *RegistrySpec) {
				// Should have imagePullSecrets
				if len(pod.Spec.ImagePullSecrets) != 1 {
					t.Errorf("expected 1 imagePullSecret, got %d", len(pod.Spec.ImagePullSecrets))
				}
				// Image should be normalized with registry URL
				expectedImage := "registry.example.com/myorg/myapp:latest"
				if pod.Spec.Containers[0].Image != expectedImage {
					t.Errorf("expected image %s, got %s", expectedImage, pod.Spec.Containers[0].Image)
				}
			},
		},
		{
			name: "pod with normalized private registry image",
			spec: PodSpec{
				Name:  "normalized-app",
				Image: "nginx:latest",
			},
			registry: &RegistrySpec{
				CredentialsSecret: "my-registry-creds",
				URL:               "registry.example.com",
			},
			wantErr: false,
			check: func(t *testing.T, pod *v1.Pod, registry *RegistrySpec) {
				// Should have imagePullSecrets since all images are private
				if len(pod.Spec.ImagePullSecrets) != 1 {
					t.Errorf("expected 1 imagePullSecret, got %d", len(pod.Spec.ImagePullSecrets))
				}
				// Should use IfNotPresent policy
				if pod.Spec.Containers[0].ImagePullPolicy != v1.PullIfNotPresent {
					t.Errorf("expected ImagePullPolicy IfNotPresent, got %v", pod.Spec.Containers[0].ImagePullPolicy)
				}
				// Image should be normalized with registry URL
				expectedImage := "registry.example.com/nginx:latest"
				if pod.Spec.Containers[0].Image != expectedImage {
					t.Errorf("expected image %s, got %s", expectedImage, pod.Spec.Containers[0].Image)
				}
			},
		},
		{
			name: "pod without registry config",
			spec: PodSpec{
				Name:  "no-registry-app",
				Image: "registry.example.com/myorg/myapp:v1.0.0",
			},
			registry: nil,
			wantErr:  false,
			check: func(t *testing.T, pod *v1.Pod, registry *RegistrySpec) {
				// Should NOT have imagePullSecrets without registry config
				if len(pod.Spec.ImagePullSecrets) != 0 {
					t.Errorf("expected 0 imagePullSecrets without registry config, got %d", len(pod.Spec.ImagePullSecrets))
				}
				// Image should remain unchanged
				expectedImage := "registry.example.com/myorg/myapp:v1.0.0"
				if pod.Spec.Containers[0].Image != expectedImage {
					t.Errorf("expected image %s, got %s", expectedImage, pod.Spec.Containers[0].Image)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod, err := mgr.createPodFromSpec("test-namespace", tt.spec, tt.registry)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && pod == nil {
				t.Error("expected pod, got nil")
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, pod, tt.registry)
			}
		})
	}
}

func TestK8sManager_ImageDetection(t *testing.T) {
	mgr := &k8sManager{}

	tests := []struct {
		name           string
		image          string
		registryURL    string
		expectedPrivate bool
	}{
		{
			name:           "Any image with registry URL",
			image:          "nginx",
			registryURL:    "registry.example.com",
			expectedPrivate: true,
		},
		{
			name:           "Full registry path",
			image:          "registry.example.com/myorg/myapp:v1.0.0",
			registryURL:    "registry.example.com",
			expectedPrivate: true,
		},
		{
			name:           "Relative image path",
			image:          "myorg/myapp:latest",
			registryURL:    "registry.example.com",
			expectedPrivate: true,
		},
		{
			name:           "Image without registry URL",
			image:          "myapp:latest",
			registryURL:    "",
			expectedPrivate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isPrivate := mgr.isPrivateRegistryImage(tt.image, tt.registryURL)
			
			if isPrivate != tt.expectedPrivate {
				t.Errorf("expected private=%v, got private=%v for image %s with registryURL %s", 
					tt.expectedPrivate, isPrivate, tt.image, tt.registryURL)
			}
		})
	}
}

func TestK8sManager_ImageNormalization(t *testing.T) {
	mgr := &k8sManager{}

	tests := []struct {
		name         string
		image        string
		registryURL  string
		expectedImage string
	}{
		{
			name:          "Simple image normalized with registry URL",
			image:         "nginx:latest",
			registryURL:   "registry.example.com",
			expectedImage: "registry.example.com/nginx:latest",
		},
		{
			name:          "Full registry path unchanged",
			image:         "registry.example.com/myorg/myapp:v1.0.0",
			registryURL:   "registry.example.com",
			expectedImage: "registry.example.com/myorg/myapp:v1.0.0",
		},
		{
			name:          "Relative image normalized",
			image:         "myorg/myapp:latest",
			registryURL:   "registry.example.com",
			expectedImage: "registry.example.com/myorg/myapp:latest",
		},
		{
			name:          "No registry URL provided",
			image:         "myorg/myapp:latest",
			registryURL:   "",
			expectedImage: "myorg/myapp:latest",
		},
		{
			name:          "Different registry not normalized",
			image:         "other.registry.com/myorg/myapp:v1.0.0",
			registryURL:   "registry.example.com",
			expectedImage: "other.registry.com/myorg/myapp:v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := mgr.normalizeImage(tt.image, tt.registryURL)
			if normalized != tt.expectedImage {
				t.Errorf("expected normalized image %s, got %s", tt.expectedImage, normalized)
			}
		})
	}
}



func TestK8sManager_CompletePrivateRegistryWorkflow(t *testing.T) {
	spec := `
registry:
  credentialsSecret: "my-registry-creds"
  url: "registry.example.com"

pods:
  - name: webapp
    image: registry.example.com/myorg/webapp:v2.1.0
    ports:
      - port: 80
        targetPort: 8080
    labels:
      app: webapp
  - name: database
    image: myorg/database:latest
    ports:
      - port: 5432
        targetPort: 5432
    labels:
      app: database
  - name: cache
    image: redis:alpine
    ports:
      - port: 6379
        targetPort: 6379
    labels:
      app: cache

services:
  - name: webapp-service
    type: ClusterIP
    ports:
      - port: 80
        targetPort: 8080
    selector:
      app: webapp
`

	// Parse the YAML
	var challengeSpec ChallengeSpec
	err := yaml.Unmarshal([]byte(spec), &challengeSpec)
	if err != nil {
		t.Fatalf("failed to parse challenge spec: %v", err)
	}

	// Verify registry configuration
	if challengeSpec.Registry == nil {
		t.Fatal("expected registry configuration, got nil")
	}
	if challengeSpec.Registry.CredentialsSecret != "my-registry-creds" {
		t.Errorf("expected registry secret my-registry-creds, got %s", challengeSpec.Registry.CredentialsSecret)
	}
	if challengeSpec.Registry.URL != "registry.example.com" {
		t.Errorf("expected registry URL registry.example.com, got %s", challengeSpec.Registry.URL)
	}

	mgr := &k8sManager{}

	// Test private image detection (no longer extracting, just validating registry config)
	if challengeSpec.Registry == nil {
		t.Fatal("expected registry configuration, got nil")
	}

	// Test Pod creation with private registry
	for _, podSpec := range challengeSpec.Pods {
		pod, err := mgr.createPodFromSpec("test-namespace", podSpec, challengeSpec.Registry)
		if err != nil {
			t.Errorf("failed to create pod %s: %v", podSpec.Name, err)
			continue
		}

		// Check that all pods should have imagePullSecrets (since all images are private)
		normalizedImage := mgr.normalizeImage(podSpec.Image, challengeSpec.Registry.URL)
		
		// Should have imagePullSecrets
		if len(pod.Spec.ImagePullSecrets) != 1 {
			t.Errorf("pod %s: expected 1 imagePullSecret, got %d", podSpec.Name, len(pod.Spec.ImagePullSecrets))
		} else if pod.Spec.ImagePullSecrets[0].Name != challengeSpec.Registry.CredentialsSecret {
			t.Errorf("pod %s: expected imagePullSecret %s, got %s", 
				podSpec.Name, challengeSpec.Registry.CredentialsSecret, pod.Spec.ImagePullSecrets[0].Name)
		}
		
		// Should use normalized image
		if pod.Spec.Containers[0].Image != normalizedImage {
			t.Errorf("pod %s: expected image %s, got %s", podSpec.Name, normalizedImage, pod.Spec.Containers[0].Image)
		}
		
		// All pods should use IfNotPresent policy
		if pod.Spec.Containers[0].ImagePullPolicy != v1.PullIfNotPresent {
			t.Errorf("pod %s: expected ImagePullPolicy IfNotPresent, got %v", 
				podSpec.Name, pod.Spec.Containers[0].ImagePullPolicy)
		}
	}
}
