/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func TestParseEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr bool
	}{
		{
			name: "default values",
			env:  map[string]string{},
			want: Config{
				Provider:                        "gke",
				GCPNodeSecureBoot:               true,
				GKEMaxPodsPerNode:               DefaultGKEMaxPodsPerNode,
				NodeMinLifespan:                 10 * time.Second,
				NodepoolDeletionDelay:           30 * time.Second,
				PodResourceType:                 "google.com/tpu",
				Concurrency:                     3,
				StaticNodepoolCreateConcurrency: 3,
				StaticNodepoolCreateTimeout:     10 * time.Minute,
				StaticNodepoolDeleteConcurrency: 3,
				SliceConditionalRecreateWait:    60 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "custom values",
			env: map[string]string{
				"PROVIDER":                       "mock",
				"GCP_PROJECT_ID":                 "test-project",
				"GCP_CLUSTER_LOCATION":           "us-central1",
				"SLICE_RECREATE_CONDITIONS":      "Reason1,Reason2:'substring'",
				"NODE_MIN_LIFESPAN":              "5s",
				"STATIC_NODEPOOL_CREATE_TIMEOUT": "5m",
				"GKE_MAX_PODS_PER_NODE":          "18",
			},
			want: Config{
				Provider:                        "mock",
				GCPProjectID:                    "test-project",
				GCPClusterLocation:              "us-central1",
				SliceRecreateConditions:         []string{"Reason1", "Reason2:'substring'"},
				GCPNodeSecureBoot:               true,
				GKEMaxPodsPerNode:               18,
				NodeMinLifespan:                 5 * time.Second,
				NodepoolDeletionDelay:           30 * time.Second,
				PodResourceType:                 "google.com/tpu",
				Concurrency:                     3,
				StaticNodepoolCreateConcurrency: 3,
				StaticNodepoolCreateTimeout:     5 * time.Minute,
				StaticNodepoolDeleteConcurrency: 3,
				SliceConditionalRecreateWait:    60 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid duration",
			env: map[string]string{
				"NODE_MIN_LIFESPAN": "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()
			// Set test environment
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			got, err := ParseEnv()
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseEnv() got = %v, want %v", got, tt.want)
			}
		})
	}
}
