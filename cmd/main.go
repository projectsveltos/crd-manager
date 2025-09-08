/*
Copyright 2025. projectsveltos.io. All rights reserved.

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
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sveltoscrds "github.com/projectsveltos/crd-manager/pkg/crds"
	"github.com/projectsveltos/libsveltos/lib/deployer"
	"github.com/projectsveltos/libsveltos/lib/k8s_utils"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

const (
	appManagedByLabel = "app.kubernetes.io/managed-by"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	klog.InitFlags(nil)

	initFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(klog.Background())

	scheme, err := initScheme()
	if err != nil {
		os.Exit(1)
	}

	restConfig := ctrl.GetConfigOrDie()

	var c client.Client
	c, err = client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		werr := fmt.Errorf("failed to connect: %w", err)
		log.Fatal(werr)
	}

	ctx := ctrl.SetupSignalHandler()

	err = deploySveltosCRDs(ctx, c, setupLog)
	if err != nil {
		os.Exit(1)
	}
}

func initScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		return nil, err
	}
	if err := apiextensionsv1.AddToScheme(s); err != nil {
		return nil, err
	}
	return s, nil
}

func initFlags(fs *pflag.FlagSet) {}

func deploySveltosCRDs(ctx context.Context, c client.Client, logger logr.Logger) error {
	objs, err := deployer.CustomSplit(string(sveltoscrds.GetSveltosCRDYAML()))
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to get Sveltos CRD instances: %v", err))
		return err
	}

	var detectedErrors error
	for _, obj := range objs {
		u, err := k8s_utils.GetUnstructured([]byte(obj))
		if err != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to get default Sveltos CRD instance: %v", err))
			detectedErrors = err
			continue
		}

		logger.V(logs.LogInfo).Info(fmt.Sprintf("considering Sveltos CRD %s", u.GetName()))
		err = processCustomResourceDefinition(ctx, c, u, logger)
		if err != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to update Sveltos CRD %s instance: %v",
				u.GetName(), err))
			detectedErrors = err
		}
	}

	return detectedErrors
}

func processCustomResourceDefinition(ctx context.Context, c client.Client, u *unstructured.Unstructured,
	logger logr.Logger) error {

	customResourceDefinition := &apiextensionsv1.CustomResourceDefinition{}
	err := c.Get(ctx,
		types.NamespacedName{Name: u.GetName()},
		customResourceDefinition)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("creating Sveltos CRD %s", u.GetName()))
			return c.Create(ctx, u)
		}
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to get default Sveltos CRD instance: %v", err))
		return err
	}

	u.SetResourceVersion(customResourceDefinition.GetResourceVersion())
	if !isManagedByHelm(u) {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("updating Sveltos CRD %s", u.GetName()))
		return c.Update(ctx, u)
	}

	return nil
}

func isManagedByHelm(u *unstructured.Unstructured) bool {
	lbls := u.GetLabels()
	if lbls == nil {
		return false
	}

	_, ok := lbls[appManagedByLabel]
	return ok
}
