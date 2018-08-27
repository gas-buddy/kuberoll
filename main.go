package main

import (
	apps "k8s.io/api/apps/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/typed/apps/v1beta1"
	"k8s.io/client-go/tools/clientcmd"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubernetes/pkg/util/interrupt"
	// "github.com/golang/glog"

	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strconv"
	"time"
)

const (
	timedOutReason     = "ProgressDeadlineExceeded"
	revisionAnnotation = "deployment.kubernetes.io/revision"
)

func main() {
	var namespace, deployment, kubeconfig *string
	deployment = flag.String("deployment", "", "deployment to restart")
	namespace = flag.String("namespace", "default", "(optional) namespace that hosts the deployment")
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "(optional) absolute path to the kubeconfig file")
	}
	flag.Parse()

	if *deployment == "" {
		panic("deployment is required")
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		panic(err.Error())
	}

	getOptions := v1.GetOptions{}
	client := clientset.AppsV1beta1().Deployments(*namespace)
	d, err := client.Get(*deployment, getOptions)

	if statusError, isStatus := err.(*errors.StatusError); err != nil {
		switch {
		case errors.IsNotFound(err):
			fmt.Printf("Deployment %s in namespace %s not found\n", *deployment, *namespace)
		case isStatus:
			fmt.Printf("Error getting deployment %s in namespace %s: %v\n",
				*deployment, *namespace, statusError.ErrStatus.Message)
		default:
			panic(err.Error())
		}
		return
	}

	fmt.Printf("Current generation: %d\n", d.Generation)
	d, err = updateDeployment(client, *deployment)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("New generation: %d\n", d.Generation)
	r, err := revision(d)
	status, done, err := deploymentStatus(client, *deployment, r)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("%s", status)
	if done {
		return
	}

	rv, err := meta.NewAccessor().ResourceVersion(d)
	if err != nil {
		panic(err.Error())
	}

	listOptions := v1.ListOptions{ResourceVersion: rv}
	w, err := client.Watch(listOptions)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	intr := interrupt.New(nil, cancel)
	err = intr.Run(func() error {
		_, err := watchtools.UntilWithoutRetry(ctx, w, func(e watch.Event) (done bool, err error) {
			status, done, err := deploymentStatus(client, *deployment, r)
			if err == nil {
				fmt.Printf("%s", status)
			}
			return
		})
		return err
	})
}

func updateDeployment(c v1beta1.DeploymentInterface, d string) (*apps.Deployment, error) {
	var deployment *apps.Deployment
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := c.Get(d, v1.GetOptions{})
		if err != nil {
			return err
		}

		annotations := getOrInitialize(result.Spec.Template.GetAnnotations())
		annotations["date"] = strconv.FormatInt(time.Now().Unix(), 10)
		result.Spec.Template.SetAnnotations(annotations)
		deployment, err = c.Update(result)

		return err
	})
	return deployment, err
}

func getOrInitialize(a map[string]string) map[string]string {
	if a == nil {
		return make(map[string]string)
	}
	return a
}

// thanks, kubernetes
func revision(obj runtime.Object) (int64, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return 0, err
	}
	v, ok := acc.GetAnnotations()[revisionAnnotation]
	if !ok {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}

func getDeploymentCondition(status apps.DeploymentStatus, condType apps.DeploymentConditionType) *apps.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}

func deploymentStatus(c v1beta1.DeploymentInterface, name string, revision int64) (string, bool, error) {
	deployment, err := c.Get(name, v1.GetOptions{})
	if err != nil {
		return "", false, err
	}
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		cond := getDeploymentCondition(deployment.Status, apps.DeploymentProgressing)
		if cond != nil && cond.Reason == timedOutReason {
			return "", false, fmt.Errorf("deployment %q exceeded its progress deadline", name)
		}
		if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d out of %d new replicas have been updated...\n", name, deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas), false, nil
		}
		if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d old replicas are pending termination...\n", name, deployment.Status.Replicas-deployment.Status.UpdatedReplicas), false, nil
		}
		if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("Waiting for deployment %q rollout to finish: %d of %d updated replicas are available...\n", name, deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas), false, nil
		}
		return fmt.Sprintf("deployment %q successfully rolled over\n", name), true, nil
	}
	return fmt.Sprintf("Waiting for deployment spec update to be observed...\n"), false, nil
}
