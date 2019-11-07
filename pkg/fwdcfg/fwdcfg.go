package fwdcfg

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	restclient "k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// GetClientConfig build the ClientConfig and return rawConfig
// if cfgFilePath is set special, use it. otherwise search the
// KUBECONFIG environment variable and merge it. if KUBECONFIG env
// is blank, use the default kubeconfig in $HOME/.kube/config
func GetClientConfig(cfgFilePath string) (*clientcmdapi.Config, error) {
	configFlag := genericclioptions.NewConfigFlags(false)
	if cfgFilePath != "" {
		configFlag.KubeConfig = &cfgFilePath
	}
	configLoader := configFlag.ToRawKubeConfigLoader()
	rawConfig, err := configLoader.RawConfig()
	if err != nil {
		return nil, err
	}
	return &rawConfig, nil
}

// GetRestConfig uses the kubectl config file to connect to
// a cluster.
func GetRestConfig(cfgFilePath string, context string) (*restclient.Config, error) {

	configFlag := genericclioptions.NewConfigFlags(false)
	if cfgFilePath != "" {
		configFlag.KubeConfig = &cfgFilePath
	}
	configFlag.Context = &context
	configLoader := configFlag.ToRawKubeConfigLoader()
	restConfig, err := configLoader.ClientConfig()
	if err != nil {
		return nil, err
	}

	return restConfig, nil
}
