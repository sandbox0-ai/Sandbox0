/*
Copyright 2026.

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

package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/sandbox0-ai/infra-operator/api/v1alpha1"
)

type reconcileStep struct {
	Name                 string
	Run                  func(context.Context) error
	ConditionType        string
	SuccessReason        string
	SuccessMessage       string
	ErrorReason          string
	SkipSuccessCondition bool
	IgnoreError          bool
	ErrorResult          *ctrl.Result
}

func (r *Sandbox0InfraReconciler) runSteps(ctx context.Context, infra *infrav1alpha1.Sandbox0Infra, steps []reconcileStep) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	for _, step := range steps {
		if step.Run == nil {
			continue
		}

		if err := step.Run(ctx); err != nil {
			if step.ConditionType != "" {
				r.setCondition(ctx, infra, step.ConditionType, metav1.ConditionFalse, step.ErrorReason, err.Error())
			}
			if step.IgnoreError {
				logger.Error(err, "Step failed but was ignored", "step", step.Name)
				continue
			}

			result := ctrl.Result{RequeueAfter: requeueInterval}
			if step.ErrorResult != nil {
				result = *step.ErrorResult
			}
			return result, err
		}

		if step.ConditionType != "" && !step.SkipSuccessCondition {
			r.setCondition(ctx, infra, step.ConditionType, metav1.ConditionTrue, step.SuccessReason, step.SuccessMessage)
		}
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}
