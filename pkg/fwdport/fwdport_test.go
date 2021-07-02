package fwdport

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/txn2/kubefwd/pkg/fwdpub"
	"github.com/txn2/txeh"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"net/http"
	"testing"
)

var (
	podName     = "test-pod-name"
	namespace   = "test-namespace"
	serviceName = "test-service-name"
)

func TestPortForward_RemovesItselfFromServiceFwd_AfterPortForwardErr(t *testing.T) {
	svcFwd := &MockServiceFWD{}

	waiter := &mockPodStateWaiter{}
	waiter.
		On("WaitUntilPodRunning", mock.Anything).
		Return(&v1.Pod{Status: v1.PodStatus{Phase: v1.PodRunning}}, nil)

	hostFile, _ := txeh.NewHostsDefault()
	pfHelper := mockPortForwardHelper{}
	pfo := &PortForwardOpts{
		Out: &fwdpub.Publisher{
			PublisherName: "Services",
			Output:        false,
		},
		Service:        serviceName,
		ServiceFwd:     svcFwd,
		PodName:        podName,
		PodPort:        "8080",
		HostFile:  		&HostFileWithLock{Hosts: hostFile},
		LocalPort:      "8080",
		Namespace:      namespace,
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
		StateWaiter:    waiter,
		PortForwardHelper: &pfHelper,
	}

	pfHelper.On("RoundTripperFor", mock.Anything).Return(nil, nil, nil)
	pfHelper.On("GetPortForwardRequest", mock.Anything).Return(nil)
	pfHelper.On("NewDialer", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	pfHelper.
		On(
			"NewOnAddresses", mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).
		Return(nil, nil)

	pfHelper.On("ForwardPorts", mock.Anything).Return(errors.New("pf error"))

	svcFwd.On("GetServicePodPortForwards", pfo.String()).Return([]*PortForwardOpts{})
	svcFwd.On("RemoveServicePodByPort", mock.Anything, mock.Anything)

	err := PortForward(pfo)
	assert.NotNil(t, err)

	<- pfo.DoneChan

	svcFwd.AssertCalled(t, "RemoveServicePodByPort", pfo.String(), pfo.PodPort)
}

type mockPodStateWaiter struct {
	mock.Mock
}

func (m *mockPodStateWaiter) WaitUntilPodRunning(stopChannel <-chan struct{}) (*v1.Pod, error) {
	args := m.Called(stopChannel)
	return args.Get(0).(*v1.Pod), args.Error(1)
}

type MockServiceFWD struct {
	mock.Mock
}

func (m *MockServiceFWD) String() string {
	return m.Called().String(0)
}

func (m *MockServiceFWD) SyncPodForwards(b bool) {
	m.Called(b)
}

func (m *MockServiceFWD) ListServicePodNames() []string {
	return m.Called().Get(0).([]string)
}

func (m *MockServiceFWD) AddServicePod(pfo *PortForwardOpts) {
	m.Called(pfo)
}

func (m *MockServiceFWD) GetServicePodPortForwards(servicePodName string) []*PortForwardOpts {
	return m.Called(servicePodName).Get(0).([]*PortForwardOpts)
}

func (m *MockServiceFWD) RemoveServicePod(servicePodName string, stop bool) {
	m.Called(servicePodName)
}

func (m *MockServiceFWD) RemoveServicePodByPort(servicePodName string, podPort string, stop bool) {
	m.Called(servicePodName, podPort)
}

type mockPortForwardHelper struct {
	mock.Mock
}

func (m *mockPortForwardHelper) GetPortForwardRequest(pfo *PortForwardOpts) *restclient.Request {
	req := m.Called(pfo).Get(0)
	if req != nil {
		return req.(*restclient.Request)
	} else {
		return nil
	}
}

func (m *mockPortForwardHelper) NewOnAddresses(dialer httpstream.Dialer, addresses []string, ports []string, stopChan <-chan struct{}, readyChan chan struct{}, out, errOut io.Writer) (*portforward.PortForwarder, error) {
	args := m.Called(dialer, addresses, ports, stopChan, readyChan, out, errOut)
	pf := args.Get(0)
	if pf != nil {
		return pf.(*portforward.PortForwarder), args.Error(1)
	} else {
		return nil, args.Error(1)
	}
}

func (m *mockPortForwardHelper) RoundTripperFor(config *restclient.Config) (http.RoundTripper, spdy.Upgrader, error) {
	args := m.Called(config)
	var rt http.RoundTripper
	var upg spdy.Upgrader
	if args.Get(0) != nil {
		rt = args.Get(0).(http.RoundTripper)
	}
	if args.Get(1) != nil {
		upg = args.Get(1).(spdy.Upgrader)
	}
	return rt, upg, args.Error(2)
}

func (m *mockPortForwardHelper) NewDialer(upgrader spdy.Upgrader, client *http.Client, method string, pfRequest *restclient.Request) httpstream.Dialer {
	var d httpstream.Dialer
	args := m.Called(upgrader, client, method, pfRequest)
	if args.Get(0) != nil {
		d = args.Get(0).(httpstream.Dialer)
	}
	return d
}

func (m *mockPortForwardHelper) ForwardPorts(forwarder *portforward.PortForwarder) error {
	return m.Called(forwarder).Error(0)
}

