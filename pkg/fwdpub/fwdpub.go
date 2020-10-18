package fwdpub

import (
	"fmt"
	"strings"
)

// Publisher
type Publisher struct {
	Output        bool
	PublisherName string
	ProducerName  string
}

// MakeProducer
func (p *Publisher) MakeProducer(producer string) Publisher {
	p.ProducerName = producer
	return *p
}

// Write
func (p *Publisher) Write(b []byte) (int, error) {
	outputString := string(b)
	outputString = strings.TrimSuffix(outputString, "\n")

	if p.Output {
		fmt.Printf("Out: %s, %s, %s", p.PublisherName, p.ProducerName, outputString)
	}
	return 0, nil
}
