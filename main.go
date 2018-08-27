package main

import (
  "k8s.io/client-go/kubernetes"
  "k8s.io/client-go/tools/clientcmd"
  "k8s.io/apimachinery/pkg/apis/meta/v1"
  "k8s.io/apimachinery/pkg/api/errors"
  "k8s.io/apimachinery/pkg/api/meta"
  "k8s.io/apimachinery/pkg/runtime"
	apps "k8s.io/api/apps/v1beta1"
  "k8s.io/apimachinery/pkg/types"
  // "github.com/golang/glog"

  "path/filepath"
  "os"
  "fmt"
  "flag"
	"strconv"
  "text/template"
  "time"
  "bytes"
)

const (
  revisionAnnotation = "deployment.kubernetes.io/revision"
  annotationPatchType = types.StrategicMergePatchType
  patchTpl = `{"spec": {"template": {"metadata": {"annotations": {"date": "{{.}}"}}}}}`
)

func main() {
  var namespace, deployment, kubeconfig *string
  deployment = flag.String("deployment", "", "deployment to restart")
  namespace = flag.String("namespace", "default", "(optional) namespace that hosts the deployment")
  if home := homeDir(); home != "" {
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

  preRevision, err := revision(d)
  if  err != nil {
    panic(err.Error())
  }

  fmt.Printf("PreRevision: %d\n", preRevision)

  d, err = client.Patch(d.GetName(), annotationPatchType, patchInput())

  if err != nil {
    panic(err.Error())
  }

  fmt.Printf("Deployment: %+v\n", d)

  postRevision, err := revision(d)
  fmt.Printf("PostRevision: %d\n", postRevision)
  if postRevision <= preRevision || err != nil {
    panic(err.Error())
  }
}

func homeDir() string {
  if h := os.Getenv("HOME"); h != "" {
    return h
  }
  return os.Getenv("USERPROFILE")
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

func patchInput() []byte {
  var patch bytes.Buffer

  t := template.Must(template.New("patch").Parse(patchTpl))
  err := t.Execute(&patch, time.Now().Unix())
  if err != nil {
    panic(err.Error())
  }

  return patch.Bytes()
}
