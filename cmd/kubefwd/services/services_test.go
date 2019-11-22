package services

import (
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdcfg"
	"github.com/txn2/kubefwd/pkg/fwdhost"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/utils"
	"github.com/txn2/txeh"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// plese run it in root permission.
// test the main pipe line.
// test will create a nginx service/deployments and portforward it.
// then test to http get the service.
func TestMainPipe(t *testing.T) {

	// we can change the namespace here
	namespace := "default"
	opts := buildFwdServiceOpts(t, namespace)

	stopListenCh := make(chan struct{})
	defer close(stopListenCh)
	defer deleteTestService(t, opts.ClientSet, namespace)

	go opts.StartListen(stopListenCh)

	go testFwd(t, opts.ClientSet, opts.Wg, namespace)

	time.Sleep(2 * time.Second)
	opts.Wg.Wait()

}

// build the FwdServiceOpts struct
func buildFwdServiceOpts(t *testing.T, namespace string) *FwdServiceOpts {

	hasRoot, err := utils.CheckRoot()

	if !hasRoot {
		t.Fatal("Please run test use Root")
		if err != nil {
			t.Fatalf("Root check failure: %s", err.Error())
		}
	}

	t.Log("Start buildFwdServiceOpts test")

	hostFile, err := txeh.NewHostsDefault()
	if err != nil {
		t.Fatalf("Hostfile error: %s", err.Error())
	}

	_, err = fwdhost.BackupHostFile(hostFile)
	if err != nil {
		t.Fatalf("Error backing up hostfile: %s\n", err.Error())
	}

	// default cfgFilePath is "$HOME/.kube/config" ;
	// if you want to use other kubeconfig pls change it here ;
	cfgFilePath := ""

	// create a ConfigGetter
	configGetter := fwdcfg.NewConfigGetter()
	// build the ClientConfig
	rawConfig, err := configGetter.GetClientConfig(cfgFilePath)
	if err != nil {
		t.Fatalf("Error in get rawConfig: %s\n", err.Error())
	}

	// ipC is the class C for the local IP address
	// increment this for each cluster
	// ipD is the class D for the local IP address
	// increment this for each service in each cluster
	ipC := 27
	ipD := 1

	stopListenCh := make(chan struct{})
	defer close(stopListenCh)

	restConfig, err := configGetter.GetRestConfig(cfgFilePath, rawConfig.CurrentContext)
	if err != nil {
		t.Fatalf("Error generating REST configuration: %s\n", err.Error())
	}

	// create the k8s clientSet
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("Error creating k8s clientSet: %s\n", err.Error())
	}

	// create the k8s RESTclient
	restClient, err := configGetter.GetRESTClient()
	if err != nil {
		t.Fatalf("Error creating k8s RestClient: %s\n", err.Error())
	}
	// create the test service
	createTestService(t, clientSet, namespace)

	listOptions := metav1.ListOptions{
		LabelSelector: "app=kubefwd-test-nginx-service",
	}

	wg := &sync.WaitGroup{}

	return &FwdServiceOpts{
		Wg:           wg,
		ClientSet:    clientSet,
		Context:      rawConfig.CurrentContext,
		Namespace:    namespace,
		ListOptions:  listOptions,
		Hostfile:     &fwdport.HostFileWithLock{Hosts: hostFile},
		ClientConfig: restConfig,
		RESTClient:   restClient,
		ShortName:    true,
		Remote:       false,
		IpC:          byte(ipC),
		IpD:          ipD,
		ExitOnFail:   exitOnFail,
		Domain:       domain,
	}
}

// create a test nginx service and deployments
func createTestService(t *testing.T, clientset *kubernetes.Clientset, namespace string) {

	// create the test nginx deployements
	// default namespace is "default"

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubefwd-test-nginx-deployment",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "kubefwd-test-nginx-deployment",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "kubefwd-test-nginx-deployment",
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "kubefwd-test-nginx-deployment",
					},
				},
				Spec: apiv1.PodSpec{
					Containers: []apiv1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.11.13-alpine",
							Ports: []apiv1.ContainerPort{
								{
									Name:          "http",
									Protocol:      apiv1.ProtocolTCP,
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.AppsV1().Deployments(namespace).Create(deployment)
	if err != nil {
		t.Fatalf("Error creating the test nginx deployment: %s\n", err.Error())
	}

	// create the test nginx service
	// default namespace is "default"
	service := &apiv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubefwd-test-nginx-service",
			Namespace: namespace,
		},
		Spec: apiv1.ServiceSpec{
			Selector: map[string]string{
				"app": "kubefwd-test-nginx-deployment",
			},
			Ports: []apiv1.ServicePort{
				{
					Protocol: apiv1.ProtocolTCP,
					Port:     80,
					TargetPort: intstr.IntOrString{
						IntVal: 80,
					},
				},
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(service)
	if err != nil {
		t.Fatalf("Error creating the test nginx deployment: %s\n", err.Error())
	}
	t.Log("Create test nginx service and deployment success")
}

// delete the test nginx service and deployments
func deleteTestService(t *testing.T, clientset *kubernetes.Clientset, namespace string) {
	clientset.AppsV1().Deployments(namespace).Delete("kubefwd-test-nginx-deployment", &metav1.DeleteOptions{})
	clientset.CoreV1().Services(namespace).Delete("kubefwd-test-nginx-service", &metav1.DeleteOptions{})
	t.Log("Delete test nginx service and deployment success")
}

// http get to test if the forward is success
func testFwd(t *testing.T, clientset *kubernetes.Clientset, wg *sync.WaitGroup, namespace string) {
	pod := findFirstPodOfService(t, clientset, namespace)
	if waitPodRunning(t, clientset, pod, namespace) {
		resp, err := http.Get("http://kubefwd-test-nginx-service/")
		if err != nil {
			t.Fatalf("Forward Test nginx service faild, http get is err: %s", err.Error())
		}
		if resp.StatusCode == 200 {
			t.Log("Kubefwd PortForward Service success!")
			os.Exit(0)
			return
		}
	}

}

func findFirstPodOfService(t *testing.T, clientset *kubernetes.Clientset, namespace string) *apiv1.Pod {
	pods, err := clientset.CoreV1().Pods(namespace).List(metav1.ListOptions{
		LabelSelector: "app=kubefwd-test-nginx-deployment",
	})
	if err != nil {
		t.Fatalf("Error get pod from Service, err: %s", err.Error())
	}
	return &pods.Items[0]
}

func waitPodRunning(t *testing.T, clientset *kubernetes.Clientset, pod *apiv1.Pod, namespace string) bool {

	if pod.Status.Phase == apiv1.PodRunning {
		return true
	}

	watcher, err := clientset.CoreV1().Pods(namespace).Watch(metav1.SingleObject(pod.ObjectMeta))
	if err != nil {
		t.Fatalf("error in create pod watcher, err: %s", err.Error())
	}
	RunningChannel := make(chan struct{})

	defer close(RunningChannel)

	go func() {
		defer watcher.Stop()
		select {
		case <-RunningChannel:
		case <-time.After(time.Second * 330):
		}
	}()

	// watcher until the pod status is running
	for {
		event := <-watcher.ResultChan()
		if event.Object != nil {
			changedPod := event.Object.(*apiv1.Pod)
			if changedPod.Status.Phase == apiv1.PodRunning {
				return true
			}
		}
		time.Sleep(time.Second * 3)
	}
}

func int32Ptr(i int32) *int32 { return &i }
