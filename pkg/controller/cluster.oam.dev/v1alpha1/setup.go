/*
Copyright 2025 The KubeVela Authors.

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

package v1alpha1

import (
	ctrl "sigs.k8s.io/controller-runtime"

	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/cluster.oam.dev/v1alpha1/clusterplane"
)

// Setup sets up all cluster.oam.dev/v1alpha1 controllers
func Setup(mgr ctrl.Manager, args controller.Args) error {
	for _, setup := range []func(ctrl.Manager, controller.Args) error{
		clusterplane.Setup,
		// Future controllers will be added here:
		// clusterplanerevision.Setup,
		// clusterblueprint.Setup,
	} {
		if err := setup(mgr, args); err != nil {
			return err
		}
	}
	return nil
}
