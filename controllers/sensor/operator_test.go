/*
Copyright 2018 BlackRock, Inc.

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

package sensor

import (
	"github.com/argoproj/argo-events/common"
	"github.com/argoproj/argo-events/pkg/apis/sensor/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestSensorOperateLifecycle(t *testing.T) {
	controller := sensorController()

	// create a new sensor object
	sensor, err := getSensor()
	assert.Nil(t, err)
	assert.NotNil(t, sensor)

	// create sensor resource
	sensor, err = controller.sensorClientset.ArgoprojV1alpha1().Sensors(sensor.Namespace).Create(sensor)
	assert.Nil(t, err)
	assert.NotNil(t, sensor)

	sOpCtx := newSensorOperationCtx(sensor, controller)
	err = sOpCtx.operate()
	assert.Nil(t, err)
	assert.Equal(t, string(v1alpha1.NodePhaseActive), string(sOpCtx.s.Status.Phase))
	for _, signal := range sOpCtx.s.Spec.Signals {
		node := getNodeByName(sOpCtx.s, signal.Name)
		assert.Equal(t, string(v1alpha1.NodePhaseActive), string(node.Phase))
	}
	// check whether sensor deployment is created
	deployment, err := controller.kubeClientset.AppsV1().Deployments(sOpCtx.s.Namespace).Get(sOpCtx.s.Name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, deployment)

	// check whether sensor service is created
	svc, err := controller.kubeClientset.CoreV1().Services(sOpCtx.s.Namespace).Get(common.DefaultSensorServiceName(sOpCtx.s.Name), metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, svc)

	// mark sensor as complete by marking all nodes as complete
	for _, signal := range sOpCtx.s.Spec.Signals {
		node := getNodeByName(sOpCtx.s, signal.Name)
		sOpCtx.markNodePhase(node.Name, v1alpha1.NodePhaseComplete, "signal is completed")
	}

	for _, signal := range sOpCtx.s.Spec.Triggers {
		node := getNodeByName(sOpCtx.s, signal.Name)
		sOpCtx.markNodePhase(node.Name, v1alpha1.NodePhaseComplete, "trigger is completed")
	}

	err = sOpCtx.operate()
	assert.Nil(t, err)
	assert.NotNil(t, sOpCtx.s)
	assert.Equal(t, string(v1alpha1.NodePhaseComplete), string(sOpCtx.s.Status.Phase))

	// check if sensor has rerun
	err = sOpCtx.operate()
	assert.Nil(t, err)
	assert.NotNil(t, sOpCtx.s)
	assert.Equal(t, string(v1alpha1.NodePhaseNew), string(sOpCtx.s.Status.Phase))

	err = sOpCtx.operate()
	assert.Nil(t, err)

	for _, signal := range sOpCtx.s.Spec.Signals {
		node := getNodeByName(sOpCtx.s, signal.Name)
		assert.Equal(t, string(v1alpha1.NodePhaseActive), string(node.Phase))
	}

	// mark sensor as error and check if it is escalated through k8 event
	sOpCtx.markSensorPhase(v1alpha1.NodePhaseError, false, "sensor is in error state")
	err = sOpCtx.operate()
	assert.Nil(t, err)
	assert.Equal(t, string(v1alpha1.NodePhaseError), string(sOpCtx.s.Status.Phase))

	sOpCtx.s.Spec.Repeat = false
	sOpCtx.markSensorPhase(v1alpha1.NodePhaseNew, false, "sensor is in new state")
	err = sOpCtx.operate()
	assert.Nil(t, err)
	assert.Equal(t, string(v1alpha1.NodePhaseActive), string(sOpCtx.s.Status.Phase))

	job, err := controller.kubeClientset.BatchV1().Jobs(sOpCtx.s.Namespace).Get(sOpCtx.s.Name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, job)
}
