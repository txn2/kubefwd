package fwdcfg

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"
	restclient "k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type ConfigGetter struct {
	ConfigFlag *genericclioptions.ConfigFlags
}

func NewConfigGetter() *ConfigGetter {
	configFlag := genericclioptions.NewConfigFlags(false)
	return &ConfigGetter{
		ConfigFlag: configFlag,
	}
}

// GetClientConfig build the ClientConfig and return rawConfig
// if cfgFilePath is set special, use it. otherwise search the
// KUBECONFIG environment variable and merge it. if KUBECONFIG env
// is blank, use the default kubeconfig in $HOME/.kube/config
func (c *ConfigGetter) GetClientConfig(cfgFilePath string) (*clientcmdapi.Config, error) {
	if cfgFilePath != "" {
		c.ConfigFlag.KubeConfig = &cfgFilePath
	}
	configLoader := c.ConfigFlag.ToRawKubeConfigLoader()
	rawConfig, err := configLoader.RawConfig()
	if err != nil {
		return nil, err
	}
	return &rawConfig, nil
}

// GetRestConfig uses the kubectl config file to connect to
// a cluster.
func (c *ConfigGetter) GetRestConfig(cfgFilePath string, context string) (*restclient.Config, error) {
	if cfgFilePath != "" {
		c.ConfigFlag.KubeConfig = &cfgFilePath
	}
	c.ConfigFlag.Context = &context
	configLoader := c.ConfigFlag.ToRawKubeConfigLoader()
	restConfig, err := configLoader.ClientConfig()
	if err != nil {
		return nil, err
	}

	return restConfig, nil
}

// GetRestClient return the RESTClient
func (c *ConfigGetter) GetRESTClient() (*restclient.RESTClient, error) {
	matchVersionKubeConfigFlags := cmdutil.NewMatchVersionFlags(c.ConfigFlag)
	f := cmdutil.NewFactory(matchVersionKubeConfigFlags)
	RESTClient, err := f.RESTClient()
	if err != nil {
		return nil, err
	}
	return RESTClient, nil
}
