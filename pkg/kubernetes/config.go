package kubernetes

import (
  "os"

  "github.com/spf13/pflag"
  "k8s.io/client-go/tools/clientcmd"
)

// NewClientConfigFlagSet returns a clientconfig and associated flagset for the config
func NewClientConfigFlagSet(prefix string) (cc clientcmd.ClientConfig, fs *pflag.FlagSet) {
  fs = pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
  rules := clientcmd.NewDefaultClientConfigLoadingRules()
  rules.DefaultClientConfig = &clientcmd.DefaultClientConfig
  overrides := clientcmd.ConfigOverrides{}
  flags := clientcmd.RecommendedConfigOverrideFlags(prefix)
  fs.StringVar(&rules.ExplicitPath, "kubeconfig", "", "Path to kubeconfig")
  clientcmd.BindOverrideFlags(&overrides, fs, flags)
  cc = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &overrides)
  return
}
