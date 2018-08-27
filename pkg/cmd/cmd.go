package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/jasisk/kuberoll/pkg/kubernetes"
	"github.com/jasisk/kuberoll/pkg/kubernetes/deployments"
	flag "github.com/spf13/pflag"
	awatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/watch"
)

var (
	noWait bool
	help   bool
)

func init() {
	flag.BoolVarP(&noWait, "no-wait", "W", false, "do not wait for the set to go out")
	flag.BoolVarP(&help, "help", "h", false, "print help text and exit")
}

type Command struct {
	Context context.Context
}

func New(ctx context.Context) *Command {
	return &Command{ctx}
}

// Run starts the command processes
func (c *Command) Run() (err error) {
	cc, fs := kubernetes.NewClientConfigFlagSet("")

	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()
	flag.Usage = usage

	if help {
		flag.Usage()
		return
	}

	name := deploymentName()
	namespace, _, err := cc.Namespace()

	if err != nil {
		return fmt.Errorf("unable to determine the cluster context")
	}

	if name == "" {
		return fmt.Errorf("deployment name is missing (add --help for usage)")
	}

	kc, err := kubernetes.NewClient(cc)

	if err != nil {
		return
	}

	dc := &deployments.Client{kc, namespace}
	d, err := dc.Get(name)
	if err != nil {
		return
	}

	fmt.Printf("%s: %s current generation: %d\n", os.Args[0], name, d.Generation)

	ctx, cancel := context.WithCancel(c.Context)
	defer cancel()

	if err = d.Annotate(); err != nil {
		return
	}

	fmt.Printf("%s: %s new generation: %d\n", os.Args[0], name, d.Generation)

	watcher, err := d.Watch()
	if err != nil {
		return
	}

	printStatus := printStatusFactory(d)

	if done, err := printStatus(); done || err != nil {
		return err
	}

	_, err = watch.UntilWithoutRetry(ctx, watcher, func(_ awatch.Event) (bool, error) {
		return printStatus()
	})
	return
}

func printStatusFactory(d *deployments.Deployment) func() (bool, error) {
	var lastStatus string
	return func() (done bool, err error) {
		status, done, err := d.CurrentStatus()
		if err == nil && status != lastStatus {
			fmt.Printf("%s: %s", os.Args[0], status)
		}
		lastStatus = status
		return

	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] deployment-name\n\n", os.Args[0])
	flag.PrintDefaults()
}

func deploymentName() (deployment string) {
	args := flag.Args()

	if len(args) != 0 {
		deployment = args[0]
	}

	return
}
