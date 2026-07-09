// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runtime

import (
	"context"
	"testing"

	"github.com/pdlc-os/fabric/pkg/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesRuntime_Run_Annotations(t *testing.T) {
	clientset := k8sfake.NewClientset()
	scheme := k8sruntime.NewScheme()
	fc := fake.NewSimpleDynamicClient(scheme)
	client := k8s.NewTestClient(fc, clientset)
	r := NewKubernetesRuntime(client)

	config := RunConfig{
		Name:         "test-agent",
		UnixUsername: "testuser",
		HomeDir:      "/home/localuser",
		Workspace:    "/path/to/workspace",
		Labels: map[string]string{
			"fabric.name": "test-agent",
		},
	}

	// Simulate logic in Run()
	if config.Workspace != "" {
		if config.Annotations == nil {
			config.Annotations = make(map[string]string)
		}
		config.Annotations["fabric.workspace"] = config.Workspace
	}

	if config.HomeDir != "" {
		if config.Annotations == nil {
			config.Annotations = make(map[string]string)
		}
		config.Annotations["fabric.homedir"] = config.HomeDir
		config.Annotations["fabric.username"] = config.UnixUsername
	}

	pod, _ := r.buildPod("default", config)

	if pod.Annotations["fabric.workspace"] != "/path/to/workspace" {
		t.Errorf("expected workspace annotation /path/to/workspace, got %s", pod.Annotations["fabric.workspace"])
	}
	if pod.Annotations["fabric.homedir"] != "/home/localuser" {
		t.Errorf("expected homedir annotation /home/localuser, got %s", pod.Annotations["fabric.homedir"])
	}
	if pod.Annotations["fabric.username"] != "testuser" {
		t.Errorf("expected username annotation testuser, got %s", pod.Annotations["fabric.username"])
	}
}

func TestKubernetesRuntime_List_Annotations(t *testing.T) {
	clientset := k8sfake.NewClientset()
	scheme := k8sruntime.NewScheme()
	fc := fake.NewSimpleDynamicClient(scheme)
	client := k8s.NewTestClient(fc, clientset)
	r := NewKubernetesRuntime(client)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
			Labels: map[string]string{
				"fabric.name": "test-agent",
			},
			Annotations: map[string]string{
				"fabric.workspace": "/path/to/workspace",
				"fabric.homedir":   "/home/localuser",
				"fabric.username":  "testuser",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Image: "test-image",
				},
			},
		},
	}

	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	agents, err := r.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
		return
	}

	if agents[0].Annotations["fabric.workspace"] != "/path/to/workspace" {
		t.Errorf("expected workspace annotation /path/to/workspace, got %s", agents[0].Annotations["fabric.workspace"])
	}
	if agents[0].Annotations["fabric.homedir"] != "/home/localuser" {
		t.Errorf("expected homedir annotation /home/localuser, got %s", agents[0].Annotations["fabric.homedir"])
	}
	if agents[0].Annotations["fabric.username"] != "testuser" {
		t.Errorf("expected username annotation testuser, got %s", agents[0].Annotations["fabric.username"])
	}
}
