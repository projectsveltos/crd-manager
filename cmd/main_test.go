/*
Copyright 2026. projectsveltos.io. All rights reserved.

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

package main

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	crdName                = "widgets.example.io"
	desiredLabelKey        = "foo"
	desiredLabelVal        = "bar"
	initialResourceVersion = "1"
)

func TestMain_(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Main Suite")
}

func testLogger() logr.Logger {
	return logr.Discard()
}

func newDesiredCRD(name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("apiextensions.k8s.io/v1")
	u.SetKind("CustomResourceDefinition")
	u.SetName(name)
	return u
}

var _ = Describe("processCustomResourceDefinition", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		var err error
		scheme, err = initScheme()
		Expect(err).NotTo(HaveOccurred())
	})

	It("leaves a Helm managed CRD untouched", func() {
		existing := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:            crdName,
				ResourceVersion: initialResourceVersion,
				Labels:          map[string]string{appManagedByLabel: "Helm"},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		Expect(processCustomResourceDefinition(context.Background(), c, newDesiredCRD(crdName),
			testLogger())).To(Succeed())

		got := &apiextensionsv1.CustomResourceDefinition{}
		Expect(c.Get(context.Background(), client.ObjectKey{Name: crdName}, got)).To(Succeed())
		Expect(got.ResourceVersion).To(Equal(initialResourceVersion))
	})

	It("updates a non Helm managed CRD with the desired changes", func() {
		existing := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:            crdName,
				ResourceVersion: initialResourceVersion,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

		desired := newDesiredCRD(crdName)
		desired.SetLabels(map[string]string{desiredLabelKey: desiredLabelVal})

		Expect(processCustomResourceDefinition(context.Background(), c, desired, testLogger())).To(Succeed())

		got := &apiextensionsv1.CustomResourceDefinition{}
		Expect(c.Get(context.Background(), client.ObjectKey{Name: crdName}, got)).To(Succeed())
		Expect(got.Labels).To(HaveKeyWithValue(desiredLabelKey, desiredLabelVal))
	})

	It("creates a missing CRD", func() {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		Expect(processCustomResourceDefinition(context.Background(), c, newDesiredCRD(crdName),
			testLogger())).To(Succeed())

		got := &apiextensionsv1.CustomResourceDefinition{}
		Expect(c.Get(context.Background(), client.ObjectKey{Name: crdName}, got)).To(Succeed())
	})
})
