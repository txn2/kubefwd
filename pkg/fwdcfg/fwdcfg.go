package fwdcfg

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sCfg is the
type K8sCfg struct {
	APIVersion string `yaml:"apiVersion"`
	Clusters   []struct {
		Cluster struct {
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
			Server                   string `yaml:"server"`
		} `yaml:"cluster"`
		Name string `yaml:"name"`
	} `yaml:"clusters"`
	Contexts []struct {
		Context struct {
			Cluster   string `yaml:"cluster"`
			Namespace string `yaml:"namespace"`
			User      string `yaml:"user"`
		} `yaml:"context"`
		Name string `yaml:"name"`
	} `yaml:"contexts"`
	CurrentContext string `yaml:"current-context"`
	Kind           string `yaml:"kind"`
	Preferences    struct {
	} `yaml:"preferences"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			ClientCertificateData string `yaml:"client-certificate-data"`
			ClientKeyData         string `yaml:"client-key-data"`
		} `yaml:"user"`
	} `yaml:"users"`
}

// K8sConfig uses the kubectl config file to connect to
// a cluster.
func K8sConfig(cmd *cobra.Command, contexts []string) *restclient.Config {

	cfgFilePath := cmd.Flag("kubeconfig").Value.String()

	if cfgFilePath == "" {
		fmt.Println("No config found. Use --kubeconfig to specify one")
		os.Exit(1)
	}

	overrides := &clientcmd.ConfigOverrides{}

	if len(contexts) > 0 {
		overrides = &clientcmd.ConfigOverrides{CurrentContext: contexts[0]}
	}

	// use the current context in kubeconfig
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: cfgFilePath},
		overrides,
	).ClientConfig()
	if err != nil {
		panic(err.Error())
	}

	return restConfig
}
