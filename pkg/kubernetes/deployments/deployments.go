package deployments

import (
	"fmt"
	apps "k8s.io/api/apps/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"strconv"
	"time"
)

const (
	timedOutReason = "ProgressDeadlineExceeded"
)

type Client struct {
	Clientset *kubernetes.Clientset
	Namespace string
}

func (c *Client) Get(name string) (deployment *Deployment, err error) {
	getOptions := v1.GetOptions{}
	ns := c.Namespace
	client := c.Clientset.AppsV1beta1().Deployments(ns)
	d, err := client.Get(name, getOptions)
	if err == nil {
		deployment = &Deployment{d, c, name}
	}
	return deployment, normalizeError(ns, name, err)
}

func normalizeError(ns, name string, err error) error {
	if statusError, isStatus := err.(*errors.StatusError); err != nil {
		switch {
		case errors.IsNotFound(err):
			return fmt.Errorf("deployment %s in namespace %s not found", name, ns)
		case isStatus:
			return fmt.Errorf("error getting deployment %s in namespace %s: %v",
				name, ns, statusError.ErrStatus.Message)
		}
	}
	return err
}

type Deployment struct {
	*apps.Deployment
	client *Client
	Name   string
}

func (d *Deployment) Annotate() error {
	var deployment *apps.Deployment
	return retry.RetryOnConflict(retry.DefaultRetry, func() (err error) {
		var lastDeployment *apps.Deployment = d.Deployment
		if _, err = d.Refresh(); err != nil {
			return err
		}

		annotations := getOrInitialize(d.Deployment.Spec.Template.GetAnnotations())
		annotations["date"] = strconv.FormatInt(time.Now().Unix(), 10)
		d.Deployment.Spec.Template.SetAnnotations(annotations)
		deployment, err = d.client.Clientset.AppsV1beta1().Deployments(d.client.Namespace).Update(d.Deployment)
		if err == nil {
			d.Deployment = deployment
		} else {
			d.Deployment = lastDeployment
		}

		return err
	})
}

func (d *Deployment) Refresh() (deployment *apps.Deployment, err error) {
	result, err := d.client.Get(d.Name)
	if err != nil {
		return
	}

	d.Deployment = result.Deployment
	return d.Deployment, nil
}

func (d *Deployment) getResourceVersion() (string, error) {
	return meta.NewAccessor().ResourceVersion(d.Deployment)
}

func (d *Deployment) CurrentStatus() (string, bool, error) {
	deployment, err := d.Refresh()
	if err != nil {
		return "", false, err
	}
	if deployment.Generation <= deployment.Status.ObservedGeneration {
		cond := getDeploymentCondition(deployment.Status, apps.DeploymentProgressing)
		if cond != nil && cond.Reason == timedOutReason {
			return "", false, fmt.Errorf("deployment %q exceeded its progress deadline", d.Name)
		}
		if deployment.Spec.Replicas != nil && deployment.Status.UpdatedReplicas < *deployment.Spec.Replicas {
			return fmt.Sprintf("waiting for deployment %q rollout to finish: %d out of %d new replicas have been updated...\n", d.Name, deployment.Status.UpdatedReplicas, *deployment.Spec.Replicas), false, nil
		}
		if deployment.Status.Replicas > deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("waiting for deployment %q rollout to finish: %d old replicas are pending termination...\n", d.Name, deployment.Status.Replicas-deployment.Status.UpdatedReplicas), false, nil
		}
		if deployment.Status.AvailableReplicas < deployment.Status.UpdatedReplicas {
			return fmt.Sprintf("waiting for deployment %q rollout to finish: %d of %d updated replicas are available...\n", d.Name, deployment.Status.AvailableReplicas, deployment.Status.UpdatedReplicas), false, nil
		}
		return fmt.Sprintf("deployment %q successfully rolled over\n", d.Name), true, nil
	}
	return fmt.Sprintf("waiting for deployment spec update to be observed...\n"), false, nil
}

func (d *Deployment) Watch() (e watch.Interface, err error) {
	rv, err := d.getResourceVersion()
	if err != nil {
		return
	}

	listOptions := v1.ListOptions{ResourceVersion: rv}
	return d.client.Clientset.AppsV1beta1().Deployments(d.client.Namespace).Watch(listOptions)
}

func getOrInitialize(a map[string]string) map[string]string {
	if a == nil {
		return make(map[string]string)
	}
	return a
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
