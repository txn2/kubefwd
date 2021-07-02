package fwdport

import (
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/txn2/kubefwd/pkg/fwdpub"
	v1 "k8s.io/api/core/v1"
	"testing"
)

var (
	podName     = "test-pod-name"
	namespace   = "test-namespace"
	serviceName = "test-service-name"
)

func TestPortForward_RemovesItselfFromServiceFwd_AfterPortForwardErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svcFwd := NewMockServiceFWD(ctrl)
	waiter := NewMockPodStateWaiter(ctrl)
	hostsOperator := NewMockHostsOperator(ctrl)

	pfHelper := NewMockPortForwardHelper(ctrl)
	pfo := &PortForwardOpts{
		Out: &fwdpub.Publisher{
			PublisherName: "Services",
			Output:        false,
		},
		Service:           serviceName,
		ServiceFwd:        svcFwd,
		PodName:           podName,
		PodPort:           "8080",
		HostFile:          nil,
		LocalPort:         "8080",
		Namespace:         namespace,
		ManualStopChan:    make(chan struct{}),
		DoneChan:          make(chan struct{}),
		StateWaiter:       waiter,
		PortForwardHelper: pfHelper,
		HostsOperator:     hostsOperator,
	}

	pfHelper.EXPECT().RoundTripperFor(gomock.Any()).Return(nil, nil, nil)
	pfHelper.EXPECT().GetPortForwardRequest(gomock.Any()).Return(nil)
	hostsOperator.EXPECT().AddHosts().Times(1)
	waiter.EXPECT().WaitUntilPodRunning(gomock.Any()).Return(&v1.Pod{Status: v1.PodStatus{Phase: v1.PodRunning}}, nil)
	pfHelper.EXPECT().NewDialer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	pfHelper.EXPECT().
		NewOnAddresses(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	pfHelper.EXPECT().ForwardPorts(gomock.Any()).Return(errors.New("pf error"))

	svcFwd.EXPECT().RemoveServicePodByPort(gomock.Eq(pfo.String()), gomock.Eq(pfo.PodPort), gomock.Eq(true))
	hostsOperator.EXPECT().RemoveHosts().Times(1)
	hostsOperator.EXPECT().RemoveInterfaceAlias().Times(1)

	err := PortForward(pfo)
	assert.NotNil(t, err)

	<- pfo.DoneChan
}

func TestPortForward_OnlyClosesDownstreamChannels_WhenExternalSignalReceived(t *testing.T) {

}