package arena

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
)

// mockK8sManagerForTesting is a test-specific implementation that doesn't require a real clientset
type mockK8sManagerForTesting struct {
	k8sManager
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
			err := mgr.ValidateSpec(tt.spec)
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
			pod, err := mgr.createPodFromSpec("test-namespace", tt.spec)
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
