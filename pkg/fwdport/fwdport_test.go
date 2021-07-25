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
		ManualStopChan:    make(chan PortForwardStopOpts),
		DoneChan:          make(chan struct{}),
		StateWaiter:       waiter,
		PortForwardHelper: pfHelper,
	}
	pfErr := errors.New("pf error")

	pfHelper.EXPECT().RoundTripperFor(gomock.Any()).Return(nil, nil, nil)
	pfHelper.EXPECT().GetPortForwardRequest(gomock.Any()).Return(nil)
	hostsOperator.EXPECT().AddHosts(gomock.Eq(pfo)).Times(1)
	waiter.EXPECT().WaitUntilPodRunning(gomock.Any()).Return(&v1.Pod{Status: v1.PodStatus{Phase: v1.PodRunning}}, nil)
	pfHelper.EXPECT().NewDialer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	pfHelper.EXPECT().
		NewOnAddresses(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	pfHelper.EXPECT().ForwardPorts(gomock.Any()).Return(pfErr)

	svcFwd.EXPECT().GetHostsOperator().Return(hostsOperator).MaxTimes(3)

	svcFwd.EXPECT().RemoveServicePodByPort(gomock.Eq(pfo.String()), gomock.Eq(pfo.PodPort), gomock.Eq(false))
	hostsOperator.EXPECT().RemoveHosts(gomock.Eq(pfo)).Times(1)
	hostsOperator.EXPECT().RemoveInterfaceAlias(gomock.Eq(pfo)).Times(1)

	err := PortForward(pfo)
	assert.NotNil(t, err)
	assert.Equal(t, pfErr, err)

	<-pfo.DoneChan
	assertChannelsClosed(t,
		assertableChannel{ch:   pfo.DoneChan, name: "DoneChan"},
		assertableChannel{ch2: pfo.ManualStopChan, name: "ManualStopChan"},
	)
}

func TestPortForward_OnlyClosesDownstreamChannels_WhenErrorOnWaitUntilPodRunning(t *testing.T) {
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
		ManualStopChan:    make(chan PortForwardStopOpts),
		DoneChan:          make(chan struct{}),
		StateWaiter:       waiter,
		PortForwardHelper: pfHelper,
	}

	untilPodRunningErr := errors.New("for example, bad credentials error from clientset")

	pfHelper.EXPECT().RoundTripperFor(gomock.Any()).Return(nil, nil, nil)
	pfHelper.EXPECT().GetPortForwardRequest(gomock.Any()).Return(nil)
	waiter.EXPECT().WaitUntilPodRunning(gomock.Any()).Return(nil, untilPodRunningErr)
	svcFwd.EXPECT().GetHostsOperator().Return(hostsOperator).AnyTimes()
	svcFwd.EXPECT().RemoveServicePodByPort(gomock.Eq(pfo.String()), gomock.Eq(pfo.PodPort), gomock.Eq(false))

	err := PortForward(pfo)
	assert.NotNil(t, err)
	assert.Equal(t, untilPodRunningErr, err)

	<-pfo.DoneChan
	assertChannelsClosed(t,
		assertableChannel{ch:   pfo.DoneChan, name: "DoneChan"},
		assertableChannel{ch2: pfo.ManualStopChan, name: "ManualStopChan"},
	)
}

func assertChannelsClosed(t *testing.T, channels ...assertableChannel) {
	for _, assertableCh := range channels {
		if assertableCh.ch != nil {
			_, open := <-assertableCh.ch
			assert.False(t, open, "%s must be closed", assertableCh.name)
		}
		if assertableCh.ch2 != nil {
			_, open := <-assertableCh.ch2
			assert.False(t, open, "%s must be closed", assertableCh.name)
		}

	}
}

type assertableChannel struct {
	ch chan struct{}
	ch2 chan PortForwardStopOpts
	name string
}