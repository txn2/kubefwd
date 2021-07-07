package fwdservice

import (
	"github.com/stretchr/testify/assert"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"sync"
	"testing"
)

var (
	firstPodName     = "test-pod-name"
	firstServiceName = "test-service-name"
	firstPort        = "80"
	secondPort       = "8080"
)

func TestServiceFWD_AddServicePod(t *testing.T) {
	svcFwd := &ServiceFWD{
		NamespaceServiceLock: &sync.Mutex{},
		PortForwards:         make(map[string][]*fwdport.PortForwardOpts),
	}

	pfOnFirstPort := &fwdport.PortForwardOpts{
		Service:    firstServiceName,
		ServiceFwd: svcFwd,
		PodName:    firstPodName,
		PodPort:    firstPort,
		LocalPort:  firstPort,
	}
	pfOnSecondPort := &fwdport.PortForwardOpts{
		Service:    firstServiceName,
		ServiceFwd: svcFwd,
		PodName:    firstPodName,
		PodPort:    secondPort,
		LocalPort:  secondPort,
	}

	registeredPortForwards := svcFwd.GetServicePodPortForwards(pfOnFirstPort.String())
	assert.Len(t, registeredPortForwards, 0)

	svcFwd.AddServicePod(pfOnFirstPort)

	registeredPortForwards = svcFwd.GetServicePodPortForwards(pfOnFirstPort.String())
	assert.Len(t, registeredPortForwards, 1)
	assert.Contains(t, registeredPortForwards, pfOnFirstPort)

	svcFwd.AddServicePod(pfOnSecondPort)
	registeredPortForwards = svcFwd.GetServicePodPortForwards(pfOnFirstPort.String())
	assert.Len(t, registeredPortForwards, 2)
	assert.Contains(t, registeredPortForwards, pfOnFirstPort)
	assert.Contains(t, registeredPortForwards, pfOnSecondPort)
}

func TestServiceFWD_RemoveServicePod(t *testing.T) {
	svcFwd := &ServiceFWD{
		NamespaceServiceLock: &sync.Mutex{},
		PortForwards:         make(map[string][]*fwdport.PortForwardOpts),
	}

	pfOnFirstPort := &fwdport.PortForwardOpts{
		Service:        firstServiceName,
		ServiceFwd:     svcFwd,
		PodName:        firstPodName,
		PodPort:        firstPort,
		LocalPort:      firstPort,
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}
	pfOnSecondPort := &fwdport.PortForwardOpts{
		Service:        firstServiceName,
		ServiceFwd:     svcFwd,
		PodName:        firstPodName,
		PodPort:        secondPort,
		LocalPort:      secondPort,
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}

	svcFwd.AddServicePod(pfOnFirstPort)
	svcFwd.AddServicePod(pfOnSecondPort)

	registeredPortForwards := svcFwd.GetServicePodPortForwards(pfOnFirstPort.String())
	assert.Len(t, registeredPortForwards, 2)

	// Simulate PortForwardOpts.PortForward() finalization
	close(pfOnFirstPort.DoneChan)
	close(pfOnSecondPort.DoneChan)

	svcFwd.RemoveServicePod(pfOnFirstPort.String(), true)

	registeredPortForwards = svcFwd.GetServicePodPortForwards(pfOnFirstPort.String())
	assert.Len(t, registeredPortForwards, 0)
}

func TestServiceFwd_RemoveServicePodByPort(t *testing.T) {
	svcFwd := &ServiceFWD{
		NamespaceServiceLock: &sync.Mutex{},
		PortForwards:         make(map[string][]*fwdport.PortForwardOpts),
	}

	pfOnFirstPort := &fwdport.PortForwardOpts{
		Service:        firstServiceName,
		ServiceFwd:     svcFwd,
		PodName:        firstPodName,
		PodPort:        firstPort,
		LocalPort:      firstPort,
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}
	pfOnSecondPort := &fwdport.PortForwardOpts{
		Service:        firstServiceName,
		ServiceFwd:     svcFwd,
		PodName:        firstPodName,
		PodPort:        secondPort,
		LocalPort:      secondPort,
		ManualStopChan: make(chan struct{}),
		DoneChan:       make(chan struct{}),
	}

	svcFwd.AddServicePod(pfOnFirstPort)
	svcFwd.AddServicePod(pfOnSecondPort)

	registeredPortForwards := svcFwd.GetServicePodPortForwards(pfOnFirstPort.String())
	assert.Len(t, registeredPortForwards, 2)

	// Simulate PortForwardOpts.PortForward() finalization
	close(pfOnFirstPort.DoneChan)
	close(pfOnSecondPort.DoneChan)

	svcFwd.RemoveServicePodByPort(pfOnFirstPort.String(), firstPort, true)

	registeredPortForwards = svcFwd.GetServicePodPortForwards(pfOnFirstPort.String())
	assert.Len(t, registeredPortForwards, 1)
	assert.NotContains(t, registeredPortForwards, pfOnFirstPort)
	assert.Contains(t, registeredPortForwards, pfOnSecondPort)

	svcFwd.RemoveServicePodByPort(pfOnFirstPort.String(), secondPort, true)

	registeredPortForwards = svcFwd.GetServicePodPortForwards(pfOnFirstPort.String())
	assert.Len(t, registeredPortForwards, 0)
	assert.NotContains(t, registeredPortForwards, pfOnFirstPort)
	assert.NotContains(t, registeredPortForwards, pfOnSecondPort)
}