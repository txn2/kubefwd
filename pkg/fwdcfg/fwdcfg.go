package fwdcfg

import (
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientCfg is a configuration data structure for the
// k8s client lib.
type ClientCfg struct {
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

// GetConfig parses the config yml supplied in cfgFilePath
// and returns a populated config object.
func GetConfig(cfgFilePath string) (*ClientCfg, error) {
	clientCfg := &ClientCfg{}

	return clientCfg, nil
}

// GetRestConfig uses the kubectl config file to connect to
// a cluster.
func GetRestConfig(cfgFilePath string, contexts []string) (*restclient.Config, error) {

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
		return nil, err
	}

	return restConfig, nil
}
