package kubernetes

import (
  "k8s.io/client-go/tools/clientcmd"
  "k8s.io/client-go/kubernetes"
)

//NewClient returns a kubernetes client
func NewClient(cc clientcmd.ClientConfig) (cs *kubernetes.Clientset, err error) {
  config, err := cc.ClientConfig()
  if err != nil {
    return
  }
  cs, err = kubernetes.NewForConfig(config)
  return
}
